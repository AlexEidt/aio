package aio

import (
	"fmt"
	"testing"
)

// import (
// 	"fmt"
// 	"os"
// 	"testing"
// )

func Test(t *testing.T) {
	// audio, _ := NewAudio("dino.mp3", "s16le")
	// defer audio.Close()
	// writer, _ := NewAudioWriter("test.mp3", &Options{
	// 	Bitrate:    audio.Bitrate(),
	// 	SampleRate: audio.SampleRate(),
	// 	Channels:   audio.Channels(),
	// 	Format:     audio.Format(),
	// })
	// fmt.Println(audio.BitsPerSample())
	// defer writer.Close()

	// // player, _ := NewPlayer(audio.Channels(), audio.SampleRate(), audio.Format())
	// // defer player.Close()
	// for audio.Read() {
	// 	// player.Play(audio.Buffer())
	// 	writer.Write(audio.Buffer())
	// }
}

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

	fmt.Println(audio)

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

	fmt.Println("Audio Copying test passed.")
}
