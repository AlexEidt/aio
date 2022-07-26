package aio

import (
	"fmt"
	"os"
	"testing"
)

func assertEquals(actual, expected interface{}) {
	if expected != actual {
		panic(fmt.Sprintf("Expected %v, got %v", expected, actual))
	}
}

func TestAudioIO(t *testing.T) {
	audio, err := NewAudio("test/beach.mp3", "s16le")
	if err != nil {
		panic(err)
	}

	defer audio.Close()
	assertEquals(audio.FileName(), "test/beach.mp3")
	assertEquals(audio.SampleRate(), 48000)
	assertEquals(audio.Channels(), 2)
	assertEquals(audio.Bitrate(), 128000)
	assertEquals(audio.Duration(), 1.032)
	assertEquals(audio.Format(), "s16le")
	assertEquals(audio.Codec(), "mp3")
	assertEquals(audio.BitsPerSample(), 16)
	assertEquals(len(audio.Buffer()), 0)

	fmt.Println("Audio File IO test passed.")
}

func TestAudioBuffer(t *testing.T) {
	audio, err1 := NewAudio("test/beach.mp3", "s16le")
	if err1 != nil {
		panic(err1)
	}

	audio.Read()
	audio.Read()
	// buffer[0:10] = [209 252 172 253 82 255 5 0 94 0]
	buffer := audio.Buffer()

	assertEquals(len(buffer), audio.SampleRate()*audio.Channels()*audio.BitsPerSample()/8)
	assertEquals(buffer[0], byte(209))
	assertEquals(buffer[1], byte(252))
	assertEquals(buffer[2], byte(172))
	assertEquals(buffer[3], byte(253))
	assertEquals(buffer[4], byte(82))
	assertEquals(buffer[5], byte(255))
	assertEquals(buffer[6], byte(5))
	assertEquals(buffer[7], byte(0))
	assertEquals(buffer[8], byte(94))
	assertEquals(buffer[9], byte(0))

	fmt.Println("Audio Buffer test passed.")
}

func TestAudioPlayback(t *testing.T) {
	audio, err1 := NewAudio("test/beach.mp3", "s16le")
	if err1 != nil {
		panic(err1)
	}
	player, err2 := NewPlayer(
		audio.Channels(),
		audio.SampleRate(),
		audio.Format(),
	)
	if err2 != nil {
		panic(err2)
	}
	defer player.Close()

	for audio.Read() {
		player.Play(audio.Buffer())
	}

	fmt.Println("Audio Playback test passed.")
}

func TestAudioCopying(t *testing.T) {
	audio, err1 := NewAudio("test/beach.mp3", "s16le")
	if err1 != nil {
		panic(err1)
	}

	options := Options{
		SampleRate: audio.SampleRate(),
		Channels:   audio.Channels(),
		Format:     audio.Format(),
	}

	writer, err2 := NewAudioWriter("test/output.mp3", &options)
	if err2 != nil {
		panic(err2)
	}

	for audio.Read() {
		writer.Write(audio.Buffer())
	}

	defer writer.Close()

	os.Remove("test/output.mp3")

	fmt.Println("Audio Copying test passed.")
}
