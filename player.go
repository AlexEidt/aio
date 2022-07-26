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

func Play(filename string) error {
	cmd := exec.Command(
		"ffplay",
		"-i", filename,
		"-nodisp",
		"-autoexit",
		"-loglevel", "quiet",
	)
	if err := cmd.Start(); err != nil {
		return err
	}

	// Stop ffplay process when user presses Ctrl+C.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		if cmd != nil {
			cmd.Process.Kill()
		}
		os.Exit(1)
	}()

	cmd.Wait()

	return nil
}

type Player struct {
	pipe *io.WriteCloser // Stdin pipe for ffplay process.
	cmd  *exec.Cmd       // ffplay command.
}

func NewPlayer(channels, samplerate int, format string) (*Player, error) {
	// Check if ffmpeg and ffprobe are installed on the users machine.
	if err := checkExists("ffplay"); err != nil {
		return nil, err
	}

	match := regexp.MustCompile(`^[fsu]\d{1,2}[lb]e$`)
	if !(format == "mulaw" || format == "alaw") && len(match.FindString(format)) == 0 {
		return nil, fmt.Errorf("audio format %s is not supported", format)
	}

	cmd := exec.Command(
		"ffplay",
		"-f", format,
		"-ac", fmt.Sprintf("%d", channels),
		"-ar", fmt.Sprintf("%d", samplerate),
		"-i", "-",
		"-nodisp",
		"-autoexit",
		"-loglevel", "quiet",
	)

	player := &Player{}

	player.cmd = cmd
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	player.pipe = &pipe
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	player.cleanup()

	return player, nil
}

func (player *Player) Play(buffer []byte) error {
	total := 0
	for total < len(buffer) {
		n, err := (*player.pipe).Write(buffer[total:])
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
		(*player.pipe).Close()
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
			(*player.pipe).Close()
		}
		if player.cmd != nil {
			player.cmd.Process.Kill()
		}
		os.Exit(1)
	}()
}
