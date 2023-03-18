package aio

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

type Player struct {
	samplerate int            // Audio Sample Rate in Hz.
	channels   int            // Number of audio channels.
	format     string         // Format of audio samples.
	pipe       io.WriteCloser // Stdin pipe for ffplay process.
	cmd        *exec.Cmd      // ffplay command.
}

func (player *Player) SampleRate() int {
	return player.samplerate
}

func (player *Player) Channels() int {
	return player.channels
}

func (player *Player) Format() string {
	switch player.format {
	case "u8", "s8":
		return player.format
	default:
		return player.format[:len(player.format)-2]
	}
}

func NewPlayer(channels, samplerate int, format string) (*Player, error) {
	// Check if ffplay is installed on the users machine.
	if err := installed("ffplay"); err != nil {
		return nil, err
	}

	format = createFormat(format)
	if err := checkFormat(format); err != nil {
		return nil, err
	}

	player := &Player{
		samplerate: samplerate,
		channels:   channels,
		format:     format,
	}

	return player, nil
}

func (player *Player) init() error {
	// If user exits with Ctrl+C, stop ffplay process.
	player.cleanup()
	// ffplay command to plat an audio stream. Takes in bytes from Stdin.
	cmd := exec.Command(
		"ffplay",
		"-f", player.format,
		"-ac", fmt.Sprintf("%d", player.channels),
		"-ar", fmt.Sprintf("%d", player.samplerate),
		"-i", "-",
		"-nodisp",
		"-autoexit",
		"-loglevel", "quiet",
	)

	player.cmd = cmd

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	player.pipe = pipe

	if err := cmd.Start(); err != nil {
		return err
	}

	return nil
}

func (player *Player) Play(samples interface{}) error {
	buffer := samplesToBytes(samples)
	if buffer == nil {
		return fmt.Errorf("invalid sample data type")
	}

	// If cmd is nil, audio player has not been initialized.
	if player.cmd == nil {
		if err := player.init(); err != nil {
			return err
		}
	}

	total := 0
	for total < len(buffer) {
		n, err := player.pipe.Write(buffer[total:])
		if err != nil {
			return err
		}
		total += n
	}

	return nil
}

// Closes the pipe and stops the ffplay process.
func (player *Player) Close() {
	if player.pipe != nil {
		player.pipe.Close()
	}
	if player.cmd != nil {
		player.cmd.Wait()
	}
}

// Stops the "cmd" process running when the user presses Ctrl+C.
// https://stackoverflow.com/questions/11268943/is-it-possible-to-capture-a-ctrlc-signal-and-run-a-cleanup-function-in-a-defe.
func (player *Player) cleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		if player.pipe != nil {
			player.pipe.Close()
		}
		if player.cmd != nil {
			player.cmd.Process.Kill()
		}
		os.Exit(1)
	}()
}
