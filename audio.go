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
	channels   int            // Number of audio channels.
	bitrate    int            // Bitrate for audio encoding.
	duration   float64        // Duration of audio in seconds.
	format     string         // Format of audio samples.
	codec      string         // Codec used for video encoding.
	bps        int            // Bits per sample.
	buffer     []byte         // Raw audio data.
	pipe       *io.ReadCloser // Stdout pipe for ffmpeg process.
	cmd        *exec.Cmd      // ffmpeg command.
}

func (audio *Audio) FileName() string {
	return audio.filename
}

// Audio Sample Rate in Hz.
func (audio *Audio) SampleRate() int {
	return audio.samplerate
}

func (audio *Audio) Channels() int {
	return audio.channels
}

func (audio *Audio) Bitrate() int {
	return audio.bitrate
}

// Audio Duration in seconds.
func (audio *Audio) Duration() float64 {
	return audio.duration
}

func (audio *Audio) Format() string {
	return audio.format[:len(audio.format)-2]
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

func (audio *Audio) Samples() interface{} {
	return convertBytesToSamples(audio.buffer, len(audio.buffer)/(audio.bps/8), audio.format)
}

// Sets the buffer to the given byte array. The length of the buffer must be a multiple
// of (bytes per sample * audio channels).
func (audio *Audio) SetBuffer(buffer []byte) error {
	if len(buffer)%(audio.bps/8*audio.channels) != 0 {
		return fmt.Errorf("buffer size must be multiple of %d", audio.bps/8*audio.channels)
	}
	audio.buffer = buffer
	return nil
}

func NewAudio(filename string, options *Options) (*Audio, error) {
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

	audio := &Audio{filename: filename}

	if options == nil {
		options = &Options{}
	}

	if options.Format == "" {
		audio.format = fmt.Sprintf("s16%s", endianness())
	} else {
		audio.format = fmt.Sprintf("%s%s", options.Format, endianness())
	}

	if err := checkFormat(audio.format); err != nil {
		return nil, err
	}

	bps := int(parse(regexp.MustCompile(`\d{1,2}`).FindString(audio.format))) // Bits per sample.
	audio.bps = bps

	audio.addAudioData(audioData)

	if options.SampleRate != 0 {
		audio.samplerate = options.SampleRate
	}

	if options.Channels != 0 {
		audio.channels = options.Channels
	}

	return audio, nil
}

// Adds audio data to the Audio struct from the ffprobe output.
func (audio *Audio) addAudioData(data map[string]string) {
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

// Once the user calls Read() for the first time on a Audio struct,
// the ffmpeg command which is used to read the audio is started.
func (audio *Audio) init() error {
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

	if audio.buffer == nil {
		audio.buffer = make([]byte, audio.samplerate*audio.channels*audio.bps/8)
	}

	return nil
}

// Reads the next frame from of audio and stores it in the buffer.
// If the last frame has been read, returns false, otherwise true.
func (audio *Audio) Read() bool {
	// If cmd is nil, video reading has not been initialized.
	if audio.cmd == nil {
		if err := audio.init(); err != nil {
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
