package aio

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

func assertEquals(actual, expected interface{}) {
	if expected != actual {
		panic(fmt.Sprintf("Expected %v, got %v", expected, actual))
	}
}

func TestSamplesInt16(t *testing.T) {
	audio, err := NewAudio("test/beach.mp3", &Options{Format: "u16"})
	if err != nil {
		panic(err)
	}

	defer audio.Close()

	for audio.Read() {
		samples := audio.Samples().([]uint16)
		bytes := audio.Buffer()

		assertEquals(len(samples), len(bytes)/2)

		index := 0
		endian := endianness()
		for i := 0; i < len(bytes); i += 2 {
			var sample uint16
			if endian == "le" {
				sample = uint16(bytes[i+0]) | uint16(bytes[i+1])<<8
			} else {
				sample = uint16(bytes[i+1]) | uint16(bytes[i+0])<<8
			}
			if sample != samples[index] {
				panic("invalid sample conversion")
			}
			index++
		}
	}

	fmt.Println("Sample Conversion Test (int16) Passed")
}

func TestSamplesFloat64(t *testing.T) {
	audio, err := NewAudio("test/beach.mp3", &Options{Format: "f64"})
	if err != nil {
		panic(err)
	}

	defer audio.Close()

	for audio.Read() {
		samples := audio.Samples().([]float64)
		bytes := audio.Buffer()

		assertEquals(len(samples), len(bytes)/8)

		index := 0
		endian := endianness()
		for i := 0; i < len(bytes); i += 8 {
			var bits uint64
			if endian == "le" {
				bits = binary.LittleEndian.Uint64(bytes[i : i+8])
			} else {
				bits = binary.BigEndian.Uint64(bytes[i : i+8])
			}
			sample := math.Float64frombits(bits)
			if sample != samples[index] {
				panic("invalid sample conversion")
			}
			index++
		}
	}

	fmt.Println("Sample Conversion test (float64) passed")
}

func TestFormatParsing(t *testing.T) {
	formats := make(map[string]bool)
	formats["s16le"] = true
	formats["s16be"] = true
	formats["s24le"] = true
	formats["s24be"] = true
	formats["s32le"] = true
	formats["s32be"] = true
	formats["s8"] = true
	formats["u16le"] = true
	formats["u16be"] = true
	formats["u24le"] = true
	formats["u24be"] = true
	formats["u32le"] = true
	formats["u32be"] = true
	formats["u8"] = true
	formats["f32le"] = true
	formats["f32be"] = true
	formats["f64le"] = true
	formats["f64be"] = true
	formats["alaw"] = false
	formats["mulaw"] = false
	formats["f64"] = false
	formats["f32"] = false
	formats["u8be"] = false

	for format, expected := range formats {
		err := checkFormat(format)
		if expected != (err == nil) {
			panic(fmt.Sprintf("Format %s failed", format))
		}
	}

	fmt.Println("Format Parsing test passed")
}

func TestBufferAlignment(t *testing.T) {
	audio, err1 := NewAudio("test/beach.mp3", nil)
	if err1 != nil {
		panic(err1)
	}

	// Since this audio is represented by stereo audio with 16-bit samples
	// the byte buffer size should be a multiple of 4 (2 channels * 2 bytes per sample)
	// since one frame of audio is 4 bytes.
	err := audio.SetBuffer(make([]byte, 101))
	if err == nil {
		panic("failed SetBuffer check")
	}

	fmt.Println("Buffer Alignment test passed")
}

func TestSetBuffer(t *testing.T) {
	audio, err := NewAudio("test/beach.mp3", nil)
	if err != nil {
		panic(err)
	}

	defer audio.Close()

	audio.SetBuffer(make([]byte, 12))

	audio.Read()

	assertEquals(len(audio.Buffer()), 12)

	fmt.Println("Set Buffer test passed")
}

func TestAudioIO(t *testing.T) {
	audio, err := NewAudio("test/beach.mp3", nil)
	if err != nil {
		panic(err)
	}

	defer audio.Close()
	assertEquals(audio.FileName(), "test/beach.mp3")
	assertEquals(audio.SampleRate(), 48000)
	assertEquals(audio.Channels(), 2)
	assertEquals(audio.Bitrate(), 128000)
	assertEquals(audio.Duration(), 1.032)
	assertEquals(audio.Format(), "s16")
	assertEquals(audio.Codec(), "mp3")
	assertEquals(audio.BitsPerSample(), 16)
	assertEquals(audio.Stream(), 0)
	assertEquals(len(audio.Buffer()), 0)

	fmt.Println("Audio File IO test passed")
}

func TestAudioBuffer(t *testing.T) {
	audio, err1 := NewAudio("test/beach.mp3", nil)
	if err1 != nil {
		panic(err1)
	}

	audio.Read()
	audio.Read()
	// buffer[0:10] = [209 252 172 253 82 255 5 0 94 0]
	buffer := audio.Buffer()

	assertEquals(len(buffer), 512)
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

	fmt.Println("Audio Buffer test passed")
}

func TestAudioPlayback(t *testing.T) {
	audio, err1 := NewAudio("test/beach.mp3", nil)
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

	audio.SetBuffer(make([]byte, audio.Total()*4))

	for audio.Read() {
		player.Play(audio.Buffer())
	}

	fmt.Println("Audio Playback test passed")
}

func TestAudioCopying(t *testing.T) {
	audio, err1 := NewAudio("test/beach.mp3", &Options{Format: "s16"})
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
		samples := audio.Samples().([]int16)
		writer.Write(samples)
	}

	defer writer.Close()

	fmt.Println("Audio Copying test passed")
}

func TestAudioResampling(t *testing.T) {
	options := Options{
		SampleRate: 4000,
		Channels:   1,
		Format:     "s8",
	}
	audio, err1 := NewAudio("test/beach.mp3", &options)
	if err1 != nil {
		panic(err1)
	}

	defer audio.Close()
	assertEquals(audio.FileName(), "test/beach.mp3")
	assertEquals(audio.SampleRate(), 4000)
	assertEquals(audio.Channels(), 1)
	assertEquals(audio.Bitrate(), 128000)
	assertEquals(audio.Duration(), 1.032)
	assertEquals(audio.Format(), "s8")
	assertEquals(audio.Codec(), "mp3")
	assertEquals(audio.Total(), 4128)
	assertEquals(audio.BitsPerSample(), 8)
	assertEquals(len(audio.Buffer()), 0)

	fmt.Println("Audio Resampling test passed")
}

// Linux and MacOS allow the user to directly choose a microphone stream by index.
// Windows requires the user to give the device name.
func TestDeviceParsingWindows(t *testing.T) {
	// Sample string taken from FFmpeg wiki:
	data := parseDevices(
		`ffmpeg version N-45279-g6b86dd5... --enable-runtime-cpudetect
  libavutil      51. 74.100 / 51. 74.100
  libavcodec     54. 65.100 / 54. 65.100
  libavformat    54. 31.100 / 54. 31.100
  libavdevice    54.  3.100 / 54.  3.100
  libavfilter     3. 19.102 /  3. 19.102
  libswscale      2.  1.101 /  2.  1.101
  libswresample   0. 16.100 /  0. 16.100
[dshow @ 03ACF580] DirectShow video devices
[dshow @ 03ACF580]  "Integrated Camera"
[dshow @ 03ACF580]  "screen-capture-recorder"
[dshow @ 03ACF580] DirectShow audio devices
[dshow @ 03ACF580]  "Internal Microphone (Conexant 2"
[dshow @ 03ACF580]  "virtual-audio-capturer"
dummy: Immediate exit requested`,
	)

	assertEquals(data[0], "Internal Microphone (Conexant 2")
	assertEquals(data[1], "virtual-audio-capturer")

	fmt.Println("Device Parsing for Windows test passed")
}

func TestMicrophoneParsing(t *testing.T) {
	mic := &Microphone{}
	err := mic.getMicrophoneData(
		`Input #0, dshow, from 'audio=Microphone Array (Realtek High Definition Audio(SST))':
		Duration: N/A, start: 653436.725000, bitrate: 1411 kb/s
		Stream #0:0: Audio: pcm_s16le, 44100 Hz, stereo, s16, 1411 kb/s`,
	)

	if err != nil {
		panic(err)
	}

	assertEquals(mic.SampleRate(), int(44100))
	assertEquals(mic.Channels(), int(2))

	fmt.Println("Webcam Parsing test passed")
}

func TestMicrophone(t *testing.T) {
	stream := 0
	max := 10
	mic, err := NewMicrophone(stream, nil)
	for err != nil && stream < max {
		mic.Close()
		stream++
		mic, err = NewMicrophone(stream, nil)
	}

	if stream == max {
		fmt.Println("No Microphone Found")
		return
	}

	seconds := 0
	for mic.Read() && seconds < 3 {
		seconds++
	}

	fmt.Println("Microphone Reading test passed")
}
