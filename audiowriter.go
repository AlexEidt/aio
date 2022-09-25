package aio

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

type AudioWriter struct {
	filename   string          // Output filename.
	streamfile string          // Extra stream data filename.
	samplerate int             // Audio Sample Rate in Hz.
	channels   int             // Number of audio channels.
	bitrate    int             // Bitrate for audio encoding.
	format     string          // Format of audio samples.
	codec      string          // Codec used for video encoding.
	pipe       *io.WriteCloser // Stdout pipe of ffmpeg process.
	cmd        *exec.Cmd       // ffmpeg command.
}

func (writer *AudioWriter) FileName() string {
	return writer.filename
}

// File used to fill in extra stream data.
func (writer *AudioWriter) StreamFile() string {
	return writer.streamfile
}

// Audio Sample Rate in Hz.
func (writer *AudioWriter) SampleRate() int {
	return writer.samplerate
}

func (writer *AudioWriter) Channels() int {
	return writer.channels
}

// Audio Bitrate in bits/s.
func (writer *AudioWriter) Bitrate() int {
	return writer.bitrate
}

func (writer *AudioWriter) Format() string {
	switch writer.format {
	case "u8", "s8":
		return writer.format
	default:
		return writer.format[:len(writer.format)-2]
	}
}

func (writer *AudioWriter) Codec() string {
	return writer.codec
}

func NewAudioWriter(filename string, options *Options) (*AudioWriter, error) {
	// Check if ffmpeg is installed on the users machine.
	if err := installed("ffmpeg"); err != nil {
		return nil, err
	}

	if options == nil {
		options = &Options{}
	}

	writer := &AudioWriter{
		filename:   filename,
		streamfile: options.StreamFile,
		bitrate:    options.Bitrate,
		codec:      options.Codec,
	}

	writer.samplerate = 44100 // 44100 Hz sampling rate by default.
	if options.SampleRate != 0 {
		writer.samplerate = options.SampleRate
	}

	writer.channels = 2 // Stereo by default.
	if options.Channels != 0 {
		writer.channels = options.Channels
	}

	if options.Format == "" {
		writer.format = createFormat("s16") // s16 default format.
	} else {
		writer.format = createFormat(options.Format)
		if err := checkFormat(writer.format); err != nil {
			return nil, err
		}
	}

	if options.StreamFile != "" {
		if !exists(options.StreamFile) {
			return nil, fmt.Errorf("file %s does not exist", options.StreamFile)
		}
		writer.streamfile = options.StreamFile
	}

	return writer, nil
}

// Once the user calls Write() for the first time on a AudioWriter struct,
// the ffmpeg command which is used to write to the audio file is started.
func (writer *AudioWriter) init() error {
	// If user exits with Ctrl+C, stop ffmpeg process.
	writer.cleanup()
	// ffmpeg command to write to audio file. Takes in bytes from Stdin and encodes them.
	command := []string{
		"-y", // overwrite output file if it exists.
		"-loglevel", "quiet",
		"-f", writer.format,
		"-ar", fmt.Sprintf("%d", writer.samplerate),
		"-ac", fmt.Sprintf("%d", writer.channels),
		"-i", "-", // The input comes from stdin.
	}

	// Assumes "writer.file" is a container format.
	if writer.streamfile != "" {
		command = append(
			command,
			"-i", writer.streamfile,
			"-map", "0:a:0",
			"-map", "1:v?", // Add Video streams if present.
			"-c:v", "copy",
			"-map", "1:s?", // Add Subtitle streams if present.
			"-c:s", "copy",
			"-map", "1:d?", // Add Data streams if present.
			"-c:d", "copy",
			"-map", "1:t?", // Add Attachments streams if present.
			"-c:t", "copy",
			"-shortest", // Cut longest streams to match audio duration.
		)
	}

	if writer.codec != "" {
		command = append(command, "-acodec", writer.codec)
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

// Writes the given samples to the audio file.
func (writer *AudioWriter) Write(samples interface{}) error {
	buffer := samplesToBytes(samples)
	if buffer == nil {
		return fmt.Errorf("invalid sample data type")
	}

	// If cmd is nil, audio writing has not been set up.
	if writer.cmd == nil {
		if err := writer.init(); err != nil {
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
