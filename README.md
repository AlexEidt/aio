# Vidio

A simple Audio I/O library written in Go. This library relies on [FFmpeg](https://www.ffmpeg.org/), [FFProbe](https://ffmpeg.org/ffprobe.html) and [FFPlay](https://ffmpeg.org/ffplay.html) which must be downloaded before usage and added to the system path.

## Installation

```
go get github.com/AlexEidt/aio
```

## Buffers

`aio` uses `byte` buffers to transport audio data. Audio data can take on many forms, including floating point, unsigned integer and signed integer. All these types are larger than a `byte` and therefore must be split. Learn more about [available audio types](https://trac.ffmpeg.org/wiki/audio%20types) from the FFmpeg Wiki. `alaw` and `mulaw` codecs are currently not supported.

As an example, if there is stereo sound (two channels) encoded in the `s16le` (signed 16 bit integers, little endian) format with a sampling rate of `44100 Hz`, one second of audio would be

```
44100 * 2 (channels) * 2 (bytes per sample) = 176400 bytes
```

## `Audio`

`Audio` is used to read audio from files. It can also be used to gather audio metadata from a file. By default, the audio buffer has a length of

```
sample rate * channels * bits per sample
```

which corresponds to 1 second of audio data.

```go
aio.NewAudio(filename, format string) (*Audio, error)

FileName() string
SampleRate() int
Channels() int
BitRate() int
Duration() float64
Format() string
Codec() string
BitsPerSample() int
Buffer() []byte
SetBuffer(buffer []byte)

Read() bool
Close()
```

## `Microphone`

`Microphone` is similar to the `Audio` struct, the only difference being that it reads audio from the microphone. The `stream` parameter is used to specify the microphone stream index, which will differ depending on the platform. For Windows (`dshow`) and MacOS (`avfoundation`), find the stream index by entering the following command

```
ffmpeg -f [dshow | avfoundation] -list_devices true -i dummy
```

and selecting the desired stream. For linux, see [this page](https://trac.ffmpeg.org/wiki/Capture/PulseAudio) on the FFmpeg Wiki.

```go
aio.NewMicrophone(stream int, format int) (*Microphone, error)

Name() string
SampleRate() int
Channels() int
Format() string
BitsPerSample() int
Buffer() []byte
SetBuffer(buffer []byte)

Read() bool
Close()
```

## `AudioWriter`

`AudioWriter` is used to write audio to files from a `byte` buffer. It comes with an `Options` struct that can be used to specify certain metadata of the output audio file.

```go
aio.NewAudioWriter(filename string, options *aio.Options) (*AudioWriter, error)

FileName() string
SampleRate() int
Channels() int
Bitrate() int
Format() string
Codec() string

Write(frame []byte) error
Close()
```

```go
type Options struct {
	SampleRate int    // Sample rate in Hz.
	Channels   int    // Number of channels.
	Bitrate    int    // Bitrate.
	Format     string // Format of audio.
	Codec      string // Audio Codec.
	Video      string // Video file to use.
}
```

## `Player`

`Player` is used to play audio from a `byte` buffer.

```go
aio.NewPlayer(channels, samplerate int, format string) (*Player, error)

Play(buffer []byte) error
Close()
```

Additionally, files may be played directly using the `Play` function:

```go
aio.Play(filename string) error
```

## Examples

Copy `input.wav` to `output.mp3`.

```go
audio, _ := aio.NewAudio("input.wav", "s16le")

options := aio.Options{
	SampleRate: audio.SampleRate(),
	Channels:   audio.Channels(),
	Bitrate:    audio.BitRate(),
	Format:     audio.Format(),
	Codec:      audio.Codec(),
}

writer, _ := aio.NewAudioWriter("output.mp3", &options)
defer writer.Close()

for audio.Read() {
	writer.Write(audio.Buffer())
}
```

Capture 10 seconds of audio from the microphone.

```go
mic, _ := aio.NewMicrophone(0, "s16le")
defer mic.Close()

options := aio.Options{
	SampleRate: mic.SampleRate(),
	Channels:   mic.Channels(),
	Format:     mic.Format(),
}

writer, _ := aio.NewAudioWriter("output.wav", &options)
defer writer.Close()

seconds := 0
for mic.Read() && seconds < 10 {
	writer.Write(mic.Buffer())
	seconds++
}
```

Play `input.mp4`.

```go
audio, _ := aio.NewAudio("input.mp4", "s16le")
player, _ := aio.NewPlayer(
	audio.Channels(),
	audio.SampleRate(),
	audio.Format()
)
defer player.Close()

for audio.Read() {
	player.Play(audio.Buffer())
}
```

Combine `sound.wav` and `movie.mp4` into `output.mp4`.

```go
audio, _ := aio.NewAudio("sound.wav", "s16le")

options := aio.Options{
	SampleRate: audio.SampleRate(),
	Channels:   audio.Channels(),
	Bitrate:    audio.Bitrate(),
	Format:     audio.Format(),
	Codec:      audio.Codec(),
	Video:      "movie.mp4",
}

writer, _ := aio.NewAudioWriter("output.mp4", &options)
defer writer.Close()

for audio.Read() {
	writer.Write(audio.Buffer())
}
```

Play Microphone audio.

```go
mic, _ := aio.NewMicrophone(0, "s16le")
defer mic.Close()

player, _ := aio.NewPlayer(
	mic.Channels(),
	mic.SampleRate(),
	mic.Format(),
)
defer player.Close()

for mic.Read() {
	player.Play(mic.Buffer())
}
```