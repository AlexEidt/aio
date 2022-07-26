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

// Adds audio data to the Audio struct from the ffprobe output.
func addAudioData(data map[string]string, audio *Audio) {
	if samplerate, ok := data["sample_rate"]; ok {
		audio.samplerate = int(parse(samplerate))
	}
	if channels, ok := data["channels"]; ok {
		audio.channels = int(parse(channels))
	}
	if bitrate, ok := data["bit_rate"]; ok {
		audio.bitrate = int(parse(bitrate))
	}
	if duration, ok := data["duration"]; ok {
		audio.duration = float64(parse(duration))
	}
	if codec, ok := data["codec_name"]; ok {
		audio.codec = codec
	}
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

// Helper function. Array contains function.
func contains(list []string, item string) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

// Parses the microphone metadata from ffmpeg output.
func parseWebcamData(buffer []byte, mic *Microphone) {
	bufferstr := string(buffer)
	// Sample String: "Stream #0:0: Audio: pcm_s16le, 44100 Hz, stereo, s16, 1411 kb/s".
	index := strings.Index(bufferstr, "Stream #")
	if index == -1 {
		index++
	}
	bufferstr = bufferstr[index:]
	// Sample rate.
	regex := regexp.MustCompile(`\d+ Hz`)
	match := regex.FindString(bufferstr)
	if len(match) > 0 {
		mic.samplerate = int(parse(match[:len(match)-len(" Hz")]))
	}

	mic.channels = 2 // stereo by default.
	if strings.Contains(bufferstr, "stereo") {
		mic.channels = 2
	} else if strings.Contains(bufferstr, "mono") {
		mic.channels = 1
	}
}
