package aio

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"syscall"
)

type Microphone struct {
	name       string         // Microphone device name.
	samplerate int            // Audio Sample Rate in Hz.
	channels   int            // Number of audio channels. 1 = mono, 2 = stereo.
	format     string         // Format of audio.
	bps        int            // Bits per sample.
	buffer     []byte         // Raw audio data.
	pipe       *io.ReadCloser // Stdout pipe for ffmpeg process streaming microphone audio.
	cmd        *exec.Cmd      // ffmpeg command.
}

func (mic *Microphone) Name() string {
	return mic.name
}

func (mic *Microphone) SampleRate() int {
	return mic.samplerate
}

func (mic *Microphone) Channels() int {
	return mic.channels
}

func (mic *Microphone) Format() string {
	return mic.format
}

func (mic *Microphone) BitsPerSample() int {
	return mic.bps
}

func (mic *Microphone) Buffer() []byte {
	return mic.buffer
}

// Sets the framebuffer to the given byte array. Note that "buffer" must be large enough
// to store one frame of mic data.
func (mic *Microphone) SetBuffer(buffer []byte) {
	mic.buffer = buffer
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

// Get microphone meta data such as width, height, fps and codec.
func getMicrophoneData(device string, mic *Microphone) error {
	// Run command to get microphone data.
	micDeviceName, err := microphone()
	if err != nil {
		return err
	}
	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner",
		"-f", micDeviceName,
		"-i", device,
	)
	// The command will fail since we do not give a file to write to, therefore
	// it will write the meta data to Stderr.
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	// Start the command.
	if err := cmd.Start(); err != nil {
		return err
	}
	// Read ffmpeg output from Stdout.
	buffer := make([]byte, 2<<11)
	total := 0
	for {
		n, err := pipe.Read(buffer[total:])
		total += n
		if err == io.EOF {
			break
		}
	}
	// Wait for the command to finish.
	cmd.Wait()

	parseMicrophoneData(buffer[:total], mic)
	return nil
}

// Creates a new microphone struct that can read from the device with the given stream index.
func NewMicrophone(stream int, options *Options) (*Microphone, error) {
	// Check if ffmpeg is installed on the users machine.
	if err := checkExists("ffmpeg"); err != nil {
		return nil, err
	}

	var device string
	switch runtime.GOOS {
	case "linux":
		device = fmt.Sprintf("%d", stream)
	case "darwin":
		device = fmt.Sprintf(`":%d"`, stream)
	case "windows":
		// If OS is windows, we need to parse the listed devices to find which corresponds to the
		// given "stream" index.
		devices, err := getDevicesWindows()
		if err != nil {
			return nil, err
		}
		if stream >= len(devices) {
			return nil, fmt.Errorf("could not find device with index: %d", stream)
		}
		device = "audio=" + devices[stream]
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	mic := Microphone{name: device}

	if err := getMicrophoneData(device, &mic); err != nil {
		return nil, err
	}

	if options == nil {
		options = &Options{}
	}

	mic.format = "s16le" // Default format.
	if options.Format != "" {
		mic.format = options.Format
	}

	if options.SampleRate != 0 {
		mic.samplerate = options.SampleRate
	}

	if options.Channels != 0 {
		mic.channels = options.Channels
	}

	match := regexp.MustCompile(`^[fsu]\d{1,2}[lb]e$`)
	if mic.format == "mulaw" || mic.format == "alaw" || len(match.FindString(mic.format)) == 0 {
		return nil, fmt.Errorf("audio format %s is not supported", mic.format)
	}

	match = regexp.MustCompile(`\d{1,2}`)
	mic.bps = int(parse(match.FindString(mic.format))) // Bits per sample.

	return &mic, nil
}

// Once the user calls Read() for the first time on a Microphone struct,
// the ffmpeg command which is used to read the microphone device is started.
func initMicrophone(mic *Microphone) error {
	// If user exits with Ctrl+C, stop ffmpeg process.
	mic.cleanup()

	micDeviceName, err := microphone()
	if err != nil {
		return err
	}

	// Use ffmpeg to pipe microphone to stdout.
	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "quiet",
		"-f", micDeviceName,
		"-i", mic.name,
		"-f", mic.format,
		"-acodec", fmt.Sprintf("pcm_%s", mic.format),
		"-ar", fmt.Sprintf("%d", mic.samplerate),
		"-ac", fmt.Sprintf("%d", mic.channels),
		"-",
	)

	mic.cmd = cmd
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	mic.pipe = &pipe
	if err := cmd.Start(); err != nil {
		return err
	}

	mic.buffer = make([]byte, mic.samplerate*mic.channels)
	return nil
}

// Reads the next audio sample from the microphone and stores in the buffer.
func (mic *Microphone) Read() bool {
	// If cmd is nil, microphone reading has not been initialized.
	if mic.cmd == nil {
		if err := initMicrophone(mic); err != nil {
			return false
		}
	}
	total := 0
	for total < len(mic.buffer) {
		n, _ := (*mic.pipe).Read(mic.buffer[total:])
		total += n
	}
	return true
}

// Closes the pipe and stops the ffmpeg process.
func (mic *Microphone) Close() {
	if mic.pipe != nil {
		(*mic.pipe).Close()
	}
	if mic.cmd != nil {
		mic.cmd.Process.Kill()
	}
}

// Stops the "cmd" process running when the user presses Ctrl+C.
// https://stackoverflow.com/questions/11268943/is-it-possible-to-capture-a-ctrlc-signal-and-run-a-cleanup-function-in-a-defe.
func (mic *Microphone) cleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		if mic.pipe != nil {
			(*mic.pipe).Close()
		}
		if mic.cmd != nil {
			mic.cmd.Process.Kill()
		}
		os.Exit(1)
	}()
}
