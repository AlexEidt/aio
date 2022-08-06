package aio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
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
func checkExists(program string) error {
	cmd := exec.Command(program, "-version")
	errmsg := fmt.Errorf("%s is not installed", program)
	if err := cmd.Start(); err != nil {
		return errmsg
	}
	if err := cmd.Wait(); err != nil {
		return errmsg
	}
	return nil
}

// Runs ffprobe on the given file and returns a map of the metadata.
func ffprobe(filename, stype string) (map[string]string, error) {
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
	buffer := make([]byte, 2<<10)
	total := 0
	for {
		n, err := pipe.Read(buffer[total:])
		total += n
		if err == io.EOF {
			break
		}
	}
	// Wait for ffprobe command to complete.
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return parseFFprobe(buffer[:total]), nil
}

// Parse ffprobe output to fill in audio data.
func parseFFprobe(input []byte) map[string]string {
	data := make(map[string]string)
	for _, line := range strings.Split(string(input), "|") {
		if strings.Contains(line, "=") {
			keyValue := strings.Split(line, "=")
			if _, ok := data[keyValue[0]]; !ok {
				data[keyValue[0]] = keyValue[1]
			}
		}
	}
	return data
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
func parseDevices(buffer []byte) []string {
	bufferstr := string(buffer)

	index := strings.Index(strings.ToLower(bufferstr), "directshow audio device")
	if index != -1 {
		bufferstr = bufferstr[index:]
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
	for _, line := range strings.Split(strings.ReplaceAll(bufferstr, "\r\n", "\n"), "\n") {
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

// Check audio format string.
func checkFormat(format string) error {
	match := regexp.MustCompile(`^(([us]8)|([us]((16)|(24)|(32))[bl]e)|(f((32)|(64))[bl]e))$`)
	if len(match.FindString(format)) == 0 {
		return fmt.Errorf("audio format %s is not supported", format)
	}
	return nil
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
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Read list devices from Stdout.
	buffer := make([]byte, 2<<10)
	total := 0
	for {
		n, err := pipe.Read(buffer[total:])
		total += n
		if err == io.EOF {
			break
		}
	}
	cmd.Wait()
	devices := parseDevices(buffer)
	return devices, nil
}
