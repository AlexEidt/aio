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

type AudioWriter struct {
	filename   string          // Output filename.
	video      string          // Video filename.
	samplerate int             // Audio Sample Rate in Hz.
	channels   int             // Number of audio channels. 1 = mono, 2 = stereo.
	bitrate    int             // Bitrate for audio encoding.
	format     string          // Format of audio.
	codec      string          // Codec used for video encoding.
	pipe       *io.WriteCloser // Stdout pipe of ffmpeg process.
	cmd        *exec.Cmd       // ffmpeg command.
}

// Optional parameters for AudioWriter.
type Options struct {
	SampleRate int    // Sample rate in Hz.
	Channels   int    // Number of channels.
	Bitrate    int    // Bitrate.
	Format     string // Format of audio.
	Codec      string // Audio Codec.
	Video      string // Video file to use.
}

func (writer *AudioWriter) FileName() string {
	return writer.filename
}

func (writer *AudioWriter) SampleRate() int {
	return writer.samplerate
}

func (writer *AudioWriter) Channels() int {
	return writer.channels
}

func (writer *AudioWriter) Bitrate() int {
	return writer.bitrate
}

func (writer *AudioWriter) Format() string {
	return writer.format
}

func (writer *AudioWriter) Codec() string {
	return writer.codec
}

// Creates a new VideoWriter struct with default values from the Options struct.
func NewAudioWriter(filename string, options *Options) (*AudioWriter, error) {
	// Check if ffmpeg is installed on the users machine.
	if err := checkExists("ffmpeg"); err != nil {
		return nil, err
	}

	writer := &AudioWriter{
		filename: filename,
		bitrate:  options.Bitrate,
		video:    options.Video,
		codec:    options.Codec,
	}

	if options.SampleRate == 0 {
		writer.samplerate = 44100
	} else {
		writer.samplerate = options.SampleRate
	}

	if options.Channels == 0 {
		writer.channels = 1
	} else {
		writer.channels = options.Channels
	}

	if options.Format == "" {
		writer.format = "s16le"
	} else {
		match := regexp.MustCompile(`^[fsu]\d{1,2}[lb]e$`)
		if options.Format == "mulaw" || options.Format == "alaw" || len(match.FindString(options.Format)) == 0 {
			return nil, fmt.Errorf("audio format %s is not supported", options.Format)
		}

		writer.format = options.Format
	}

	if options.Video != "" {
		if !exists(options.Video) {
			return nil, fmt.Errorf("video file %s does not exist", options.Video)
		}

		videoData, err := ffprobe(options.Video, "v")
		if err != nil {
			return nil, err
		} else if len(videoData) == 0 {
			return nil, fmt.Errorf("given video file %s has no video", options.Video)
		}

		writer.video = options.Video
	}

	return writer, nil
}

// Once the user calls Write() for the first time on a VideoWriter struct,
// the ffmpeg command which is used to write to the video file is started.
func initVideoWriter(writer *AudioWriter) error {
	// If user exits with Ctrl+C, stop ffmpeg process.
	writer.cleanup()
	// ffmpeg command to write to video file. Takes in bytes from Stdin and encodes them.
	command := []string{
		"-y", // overwrite output file if it exists.
		"-loglevel", "quiet",
		"-f", writer.format,
		"-acodec", fmt.Sprintf("pcm_%s", writer.format),
		"-ar", fmt.Sprintf("%d", writer.samplerate),
		"-ac", fmt.Sprintf("%d", writer.channels),
		"-i", "-", // The input comes from stdin.
	}

	// Parameter logic from:
	// https://github.com/Zulko/moviepy/blob/18e9f57d1abbae8051b9aef75de3f19b4d1f0630/moviepy/audio/io/ffmpeg_audiowriter.py
	if writer.video != "" {
		command = append(
			command,
			"-i", writer.video,
			"-vcodec", "copy",
		)
	} else {
		command = append(command, "-vn")
	}

	if writer.codec != "" {
		command = append(
			command,
			"-acodec", writer.codec,
		)
	}

	if writer.bitrate > 0 {
		command = append(command, "-ab", fmt.Sprintf("%d", writer.bitrate))
	}

	command = append(command, writer.filename)
	cmd := exec.Command("ffmpeg", command...)
	writer.cmd = cmd

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	writer.pipe = &pipe
	if err := cmd.Start(); err != nil {
		return err
	}

	return nil
}

// Writes the given buffer to the audio file.
func (writer *AudioWriter) Write(buffer []byte) error {
	// If cmd is nil, audio writing has not been set up.
	if writer.cmd == nil {
		if err := initVideoWriter(writer); err != nil {
			return err
		}
	}
	total := 0
	for total < len(buffer) {
		n, err := (*writer.pipe).Write(buffer[total:])
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

// Closes the pipe and stops the ffmpeg process.
func (writer *AudioWriter) Close() {
	if writer.pipe != nil {
		(*writer.pipe).Close()
	}
	if writer.cmd != nil {
		writer.cmd.Wait()
	}
}

// Stops the "cmd" process running when the user presses Ctrl+C.
// https://stackoverflow.com/questions/11268943/is-it-possible-to-capture-a-ctrlc-signal-and-run-a-cleanup-function-in-a-defe.
func (writer *AudioWriter) cleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		if writer.pipe != nil {
			(*writer.pipe).Close()
		}
		if writer.cmd != nil {
			writer.cmd.Process.Kill()
		}
		os.Exit(1)
	}()
}
