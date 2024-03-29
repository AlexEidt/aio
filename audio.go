package aio

import (
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"
)

type Audio struct {
	filename   string            // Audio Filename.
	samplerate int               // Audio Sample Rate in Hz.
	channels   int               // Number of audio channels.
	bitrate    int               // Bitrate for audio encoding.
	bps        int               // Bits per sample.
	stream     int               // Stream Index.
	duration   float64           // Duration of audio in seconds.
	format     string            // Format of audio samples.
	codec      string            // Codec used for video encoding.
	ended      bool              // Flag storing whether Audio reading has ended.
	hasstreams bool              // Flag storing whether file has additional data streams.
	buffer     []byte            // Raw audio data.
	metadata   map[string]string // Audio Metadata.
	pipe       io.ReadCloser     // Stdout pipe for ffmpeg process.
	cmd        *exec.Cmd         // ffmpeg command.
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

// Audio Bitrate in bits/s.
func (audio *Audio) Bitrate() int {
	return audio.bitrate
}

func (audio *Audio) BitsPerSample() int {
	return audio.bps
}

// Returns the zero-indexed audio stream index.
func (audio *Audio) Stream() int {
	return audio.stream
}

// Returns the total number of audio samples in the file in bytes.
func (audio *Audio) Total() int {
	frame := audio.channels * audio.bps / 8
	second := audio.samplerate * frame
	total := int(math.Ceil(float64(second) * audio.duration))
	return total + (frame-total%frame)%frame
}

// Audio Duration in seconds.
func (audio *Audio) Duration() float64 {
	return audio.duration
}

func (audio *Audio) Format() string {
	switch audio.format {
	case "u8", "s8":
		return audio.format
	default:
		return audio.format[:len(audio.format)-2]
	}
}

func (audio *Audio) Codec() string {
	return audio.codec
}

// Returns true if file has any video, subtitle, data or attachment streams.
func (audio *Audio) HasStreams() bool {
	return audio.hasstreams
}

func (audio *Audio) Buffer() []byte {
	return audio.buffer
}

// Raw Metadata from ffprobe output for the audio file.
func (audio *Audio) MetaData() map[string]string {
	return audio.metadata
}

// Casts the values in the byte buffer to those specified by the audio format.
func (audio *Audio) Samples() interface{} {
	return bytesToSamples(audio.buffer, len(audio.buffer)/(audio.bps/8), audio.format)
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
	if options == nil {
		options = &Options{}
	}

	streams, err := NewAudioStreams(filename, options)
	if streams == nil {
		return nil, err
	}

	if options.Stream < 0 || options.Stream >= len(streams) {
		return nil, fmt.Errorf("invalid stream index: %d, must be between 0 and %d", options.Stream, len(streams))
	}

	return streams[options.Stream], err
}

// Read all audio streams from the given file.
func NewAudioStreams(filename string, options *Options) ([]*Audio, error) {
	if !exists(filename) {
		return nil, fmt.Errorf("video file %s does not exist", filename)
	}
	// Check if ffmpeg and ffprobe are installed on the users machine.
	if err := installed("ffmpeg"); err != nil {
		return nil, err
	}
	if err := installed("ffprobe"); err != nil {
		return nil, err
	}

	audioData, err := ffprobe(filename, "a")
	if err != nil {
		return nil, err
	}

	if len(audioData) == 0 {
		return nil, fmt.Errorf("no audio data found in %s", filename)
	}

	if options == nil {
		options = &Options{}
	}

	var format string
	if options.Format == "" {
		format = createFormat("s16") // s16 default format.
	} else {
		format = createFormat(options.Format)
	}

	if err := checkFormat(format); err != nil {
		return nil, err
	}

	bps := int(parse(regexp.MustCompile(`\d{1,2}`).FindString(format))) // Bits per sample.

	// Loop over all stream types. v: Video, s: Subtitle, d: Data, t: Attachments
	hasstream := false
	for _, c := range "vsdt" {
		data, err := ffprobe(filename, string(c))
		if err != nil {
			return nil, err
		}
		if len(data) > 0 {
			hasstream = true
			break
		}
	}

	streams := make([]*Audio, len(audioData))
	for i, data := range audioData {
		audio := &Audio{
			filename:   filename,
			format:     format,
			bps:        bps,
			stream:     i,
			hasstreams: hasstream,
			metadata:   data,
		}

		audio.addAudioData(data)

		if options.SampleRate != 0 {
			audio.samplerate = options.SampleRate
		}

		if options.Channels != 0 {
			audio.channels = options.Channels
		}

		streams[i] = audio
	}

	return streams, nil
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
		"-ar", fmt.Sprintf("%d", audio.samplerate),
		"-ac", fmt.Sprintf("%d", audio.channels),
		"-map", fmt.Sprintf("0:a:%d", audio.stream),
		"-loglevel", "quiet",
		"-",
	)

	audio.cmd = cmd

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	audio.pipe = pipe

	if err := cmd.Start(); err != nil {
		return err
	}

	if audio.buffer == nil {
		audio.buffer = make([]byte, audio.samplerate*audio.channels*audio.bps/8)
	}

	return nil
}

// Reads the next frame of audio and stores it in the buffer.
// If the last audio frame has been read, returns false, otherwise true.
func (audio *Audio) Read() bool {
	if audio.ended {
		return false
	}

	// If cmd is nil, audio reading has not been initialized.
	if audio.cmd == nil {
		if err := audio.init(); err != nil {
			return false
		}
	}

	n, err := io.ReadFull(audio.pipe, audio.buffer)

	if err != nil {
		// When the user reaches the end of the audio stream, the buffer will have to be shortened
		// such that the audio stream is accurately represented.
		// The rest of this sliced array is not garbage collected.
		audio.buffer = audio.buffer[:n]
		audio.Close()
	}

	return n > 0
}

// Closes the pipe and stops the ffmpeg process.
func (audio *Audio) Close() {
	audio.ended = true
	if audio.pipe != nil {
		audio.pipe.Close()
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
			audio.pipe.Close()
		}
		if audio.cmd != nil {
			audio.cmd.Process.Kill()
		}
		os.Exit(1)
	}()
}
