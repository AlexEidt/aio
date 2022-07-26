package aio

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"
)

type Audio struct {
	filename   string         // Audio Filename.
	samplerate int            // Audio Sample Rate in Hz.
	channels   int            // Number of audio channels. 1 = mono, 2 = stereo.
	bitrate    int            // Bitrate for audio encoding.
	duration   float64        // Duration of audio in seconds.
	format     string         // Format of audio.
	codec      string         // Codec used for video encoding.
	bps        int            // Bits per sample.
	buffer     []byte         // Raw audio data.
	pipe       *io.ReadCloser // Stdout pipe for ffmpeg process.
	cmd        *exec.Cmd      // ffmpeg command.
}

func (audio *Audio) FileName() string {
	return audio.filename
}

func (audio *Audio) SampleRate() int {
	return audio.samplerate
}

func (audio *Audio) Channels() int {
	return audio.channels
}

func (audio *Audio) Bitrate() int {
	return audio.bitrate
}

func (audio *Audio) Duration() float64 {
	return audio.duration
}

func (audio *Audio) Format() string {
	return audio.format
}

func (audio *Audio) Codec() string {
	return audio.codec
}

func (audio *Audio) BitsPerSample() int {
	return audio.bps
}

func (audio *Audio) Buffer() []byte {
	return audio.buffer
}

// Sets the framebuffer to the given byte array. Note that "buffer" must be large enough
// to store one frame of audio data.
func (audio *Audio) SetBuffer(buffer []byte) {
	audio.buffer = buffer
}

// Creates a new Audio struct.
// Uses ffprobe to get audio information and fills in the Audio struct with this data.
func NewAudio(filename, format string) (*Audio, error) {
	if !exists(filename) {
		return nil, fmt.Errorf("video file %s does not exist", filename)
	}
	// Check if ffmpeg and ffprobe are installed on the users machine.
	if err := checkExists("ffmpeg"); err != nil {
		return nil, err
	}
	if err := checkExists("ffprobe"); err != nil {
		return nil, err
	}

	audioData, err := ffprobe(filename, "a")
	if err != nil {
		return nil, err
	}

	if len(audioData) == 0 {
		return nil, fmt.Errorf("no audio data found in %s", filename)
	}

	match := regexp.MustCompile(`^[fsu]\d{1,2}[lb]e$`)
	if format == "mulaw" || format == "alaw" || len(match.FindString(format)) == 0 {
		return nil, fmt.Errorf("audio format %s is not supported", format)
	}

	match = regexp.MustCompile(`\d{1,2}`)
	bps := int(parse(match.FindString(format))) // Bits per sample.

	audio := &Audio{
		filename: filename,
		format:   format,
		bps:      bps,
	}

	addAudioData(audioData, audio)

	return audio, nil
}

// Once the user calls Read() for the first time on a Audio struct,
// the ffmpeg command which is used to read the audio is started.
func initAudio(audio *Audio) error {
	// If user exits with Ctrl+C, stop ffmpeg process.
	audio.cleanup()
	// ffmpeg command to pipe audio data to stdout.
	cmd := exec.Command(
		"ffmpeg",
		"-i", audio.filename,
		"-f", audio.format,
		"-acodec", fmt.Sprintf("pcm_%s", audio.format),
		"-ar", fmt.Sprintf("%d", audio.samplerate),
		"-ac", fmt.Sprintf("%d", audio.channels),
		"-loglevel", "quiet",
		"-",
	)

	audio.cmd = cmd
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	audio.pipe = &pipe
	if err := cmd.Start(); err != nil {
		return err
	}

	audio.buffer = make([]byte, audio.samplerate*audio.channels*audio.bps/8)

	return nil
}

// Reads the next frame from of audio and stores it in the buffer.
// If the last frame has been read, returns false, otherwise true.
func (audio *Audio) Read() bool {
	// If cmd is nil, video reading has not been initialized.
	if audio.cmd == nil {
		if err := initAudio(audio); err != nil {
			return false
		}
	}
	total := 0
	for total < len(audio.buffer) {
		n, err := (*audio.pipe).Read(audio.buffer[total:])
		if err == io.EOF {
			audio.Close()
			return false
		}
		total += n
	}
	return true
}

// Closes the pipe and stops the ffmpeg process.
func (audio *Audio) Close() {
	if audio.pipe != nil {
		(*audio.pipe).Close()
	}
	if audio.cmd != nil {
		audio.cmd.Wait()
	}
}

// Stops the "cmd" process running when the user presses Ctrl+C.
// https://stackoverflow.com/questions/11268943/is-it-possible-to-capture-a-ctrlc-signal-and-run-a-cleanup-function-in-a-defe.
func (audio *Audio) cleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		if audio.pipe != nil {
			(*audio.pipe).Close()
		}
		if audio.cmd != nil {
			audio.cmd.Process.Kill()
		}
		os.Exit(1)
	}()
}
