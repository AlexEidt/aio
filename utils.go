package aio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

// Returns true if file exists, false otherwise.
// https://stackoverflow.com/questions/12518876/how-to-check-if-a-file-exists-in-go.
func exists(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	return false
}

// Checks if the given program is installed.
func installed(program string) error {
	cmd := exec.Command(program, "-version")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s is not installed", program)
	}

	return nil
}

// Runs ffprobe on the given file and returns a map of the metadata.
func ffprobe(filename, stype string) ([]map[string]string, error) {
	// "stype" is stream stype. "v" for video, "a" for audio.
	// Extract media metadata information with ffprobe.
	cmd := exec.Command(
		"ffprobe",
		"-show_streams",
		"-select_streams", stype,
		"-print_format", "compact",
		"-loglevel", "quiet",
		filename,
	)

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Read ffprobe output from Stdout.
	builder := bytes.Buffer{}
	buffer := make([]byte, 1024)
	for {
		n, err := pipe.Read(buffer)
		builder.Write(buffer[:n])
		if err == io.EOF {
			break
		}
	}

	// Wait for ffprobe command to complete.
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	// Parse ffprobe output to fill in audio data.
	datalist := make([]map[string]string, 0)
	metadata := builder.String()
	for _, stream := range strings.Split(metadata, "\n") {
		if len(strings.TrimSpace(stream)) > 0 {
			data := make(map[string]string)
			for _, line := range strings.Split(stream, "|") {
				if strings.Contains(line, "=") {
					keyValue := strings.Split(line, "=")
					if _, ok := data[keyValue[0]]; !ok {
						data[keyValue[0]] = keyValue[1]
					}
				}
			}
			datalist = append(datalist, data)
		}
	}

	return datalist, nil
}

// Parses the given data into a float64.
func parse(data string) float64 {
	n, err := strconv.ParseFloat(data, 64)
	if err != nil {
		return 0
	}
	return n
}

// Returns the microphone device name used for the -f option with ffmpeg.
func microphone() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return "pulse", nil
	case "darwin":
		return "avfoundation", nil // qtkit
	case "windows":
		return "dshow", nil // vfwcap
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// For webcam streaming on windows, ffmpeg requires a device name.
// All device names are parsed and returned by this function.
func parseDevices(buffer string) []string {
	index := strings.Index(strings.ToLower(buffer), "directshow audio device")
	if index != -1 {
		buffer = buffer[index:]
	}

	type Pair struct {
		name string
		alt  string
	}
	// Parses ffmpeg output to get device names. Windows only.
	// Uses parsing approach from https://github.com/imageio/imageio/blob/master/imageio/plugins/ffmpeg.py#L681.

	pairs := []Pair{}
	// Find all device names surrounded by quotes. E.g "Windows Camera Front"
	regex := regexp.MustCompile("\"[^\"]+\"")
	for _, line := range strings.Split(strings.ReplaceAll(buffer, "\r\n", "\n"), "\n") {
		if strings.Contains(strings.ToLower(line), "alternative name") {
			match := regex.FindString(line)
			if len(match) > 0 {
				pairs[len(pairs)-1].alt = match[1 : len(match)-1]
			}
		} else {
			match := regex.FindString(line)
			if len(match) > 0 {
				pairs = append(pairs, Pair{name: match[1 : len(match)-1]})
			}
		}
	}

	devices := []string{}
	// If two devices have the same name, use the alternate name of the later device as its name.
	for _, pair := range pairs {
		if contains(devices, pair.name) {
			devices = append(devices, pair.alt)
		} else {
			devices = append(devices, pair.name)
		}
	}

	return devices
}

func contains(list []string, item string) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

// Returns the microphone device name.
// On windows, ffmpeg output from the -list_devices command is parsed to find the device name.
func getDevicesWindows() ([]string, error) {
	// Run command to get list of devices.
	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner",
		"-list_devices", "true",
		"-f", "dshow",
		"-i", "dummy",
	)

	pipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	// Start the command and immediately continue so that the pipe can be read.
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Read list devices from Stdout.
	builder := bytes.Buffer{}
	buffer := make([]byte, 1024)
	for {
		n, err := pipe.Read(buffer)
		builder.Write(buffer[:n])
		if err == io.EOF {
			break
		}
	}

	// Wait for the command to finish.
	cmd.Wait()

	devices := parseDevices(builder.String())
	return devices, nil
}

// Check audio format string.
func checkFormat(format string) error {
	match := regexp.MustCompile(`^(([us]8)|([us]((16)|(24)|(32))[bl]e)|(f((32)|(64))[bl]e))$`)
	if len(match.FindString(format)) == 0 {
		formats := "u8, s8, u16, s16, u24, s24, u32, s32, f32, or f64"
		return fmt.Errorf("audio format %s is not supported, must be one of %s", format[:len(format)-2], formats)
	}
	return nil
}

func createFormat(format string) string {
	switch format {
	case "u8", "s8":
		return format
	default:
		return fmt.Sprintf("%s%s", format, endianness())
	}
}

// Little Endian -> "le", Big Endian -> "be".
func endianness() string {
	x := 1
	littleEndian := *(*byte)(unsafe.Pointer(&x)) == 1
	if littleEndian {
		return "le"
	} else {
		return "be"
	}
}

// Alias the byte buffer as a certain type specified by the format string.
func bytesToSamples(buffer []byte, size int, format string) interface{} {
	switch format {
	case "f32be", "f32le":
		var data []float32
		pointer := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&buffer)).Data
		pointer.Cap = size
		pointer.Len = size
		return data
	case "f64be", "f64le":
		var data []float64
		pointer := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&buffer)).Data
		pointer.Cap = size
		pointer.Len = size
		return data
	case "s16be", "s16le":
		var data []int16
		pointer := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&buffer)).Data
		pointer.Cap = size
		pointer.Len = size
		return data
	case "s32be", "s32le":
		var data []int32
		pointer := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&buffer)).Data
		pointer.Cap = size
		pointer.Len = size
		return data
	case "s8":
		var data []int8
		pointer := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&buffer)).Data
		pointer.Cap = size
		pointer.Len = size
		return data
	case "u16be", "u16le":
		var data []uint16
		pointer := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&buffer)).Data
		pointer.Cap = size
		pointer.Len = size
		return data
	case "u32be", "u32le":
		var data []uint32
		pointer := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&buffer)).Data
		pointer.Cap = size
		pointer.Len = size
		return data
	default:
		return buffer
	}
}

func samplesToBytes(data interface{}) []byte {
	var buffer []byte
	pointer := (*reflect.SliceHeader)(unsafe.Pointer(&buffer))

	var size int
	switch data := data.(type) {
	case []uint8:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data)
	case []int8:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data)
	case []uint16:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data) * 2
	case []int16:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data) * 2
	case []uint32:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data) * 4
	case []int32:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data) * 4
	case []float32:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data) * 4
	case []float64:
		pointer.Data = (*reflect.SliceHeader)(unsafe.Pointer(&data)).Data
		size = len(data) * 8
	default:
		return nil
	}

	pointer.Cap = size
	pointer.Len = size

	return buffer
}
