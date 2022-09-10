# `aio`

A simple Audio I/O library written in Go. This library relies on [FFmpeg](https://www.ffmpeg.org/), [FFProbe](https://ffmpeg.org/ffprobe.html) and [FFPlay](https://ffmpeg.org/ffplay.html) which must be downloaded before usage and added to the system path.

For Video I/O using FFmpeg. see the [`Vidio`](https://github.com/AlexEidt/Vidio) project.

## Installation

```
go get github.com/AlexEidt/aio
```

## Buffers

`aio` uses `byte` buffers to transport raw audio data. Audio data can take on many forms, including floating point, unsigned integer and signed integer. These types may be larger than a `byte` and would have to be split. Valid formats are `u8`, `s8`, `u16`, `s16`, `u24`, `s24`, `u32`, `s32`, `f32`, and `f64`. These represent `u` unsigned integers, `s` signed integers and `f` floating point numbers.

As an example, if there is stereo sound (two channels) encoded in the `s16` (signed 16 bit integers) format with a sampling rate of `44100 Hz`, one second of audio would be

```
44100 * 2 (channels) * 2 (bytes per sample) = 176400 bytes
```

Continuing on with this example, since this is stereo audio with 2 channels, one frame of audio is represented by 2 consecutive integers, one for each channel. Each integer is 16 bits, which means one frame of audio would be represented by 4 consecutive bytes.

## `Options`

The `Options` struct is used to specify optional parameters for Audio I/O.

```go
type Options struct {
	Stream     int    // Audio Stream Index to use.
	SampleRate int    // Sample rate in Hz.
	Channels   int    // Number of channels.
	Bitrate    int    // Bitrate.
	Format     string // Format of audio.
	Codec      string // Audio Codec.
	Video      string // Video file to use.
}
```

## `Audio`

`Audio` is used to read audio from files. It can also be used to gather audio metadata from a file. By default, the audio buffer has a length of

```
sample rate * channels * bytes per sample
```

which corresponds to 1 second of audio data.

The user may pass in `options` to set the desired sampling rate, format and channels of the audio. If `options` is `nil`, then the channels and sampling rate from the file will be used, with a default format of `s16`.

The `Read()` function fills the internal byte buffer with the next batch of audio samples. Once the entire file has been read, `Read()` will return `false` and close the `Audio` struct.

Note that the `Samples()` function is only present for convenience. It casts the raw byte buffer into the given audio data type determined by the `Format()` such that the underlying data buffers are the same. The `s24` and `u24` formats are not supported by the `Samples()` function since there is no type equivalent. Calling the `Samples()` function on 24-bit audio will return the raw byte buffer.

The return value of the `Samples()` function will have to be cast into an array of the desired type (e.g. `audio.Samples().([]float32)`)

```go
aio.NewAudio(filename string, options *aio.Options) (*aio.Audio, error)
aio.NewAudioStreams(filename string, options *Options) ([]*aio.Audio, error)

FileName() string
SampleRate() int
Channels() int
Bitrate() int
Duration() float64
Format() string
Codec() string
BitsPerSample() int
Stream() int
Total() int
Buffer() []byte
MetaData() map[string]string
Samples() interface{}
SetBuffer(buffer []byte) error

Read() bool
Close()
```

## `AudioWriter`

`AudioWriter` is used to write audio to files from a buffer of audio samples. It comes with an `Options` struct that can be used to specify certain metadata of the output audio file. If `options` is `nil`, the defaults used are a sampling rate of `44100 Hz`, with `2` channels in the `s16` format.

```go
aio.NewAudioWriter(filename string, options *aio.Options) (*aio.AudioWriter, error)

FileName() string
SampleRate() int
Channels() int
Bitrate() int
Format() string
Codec() string
Video() string

Write(samples interface{}) error
Close()
```

## `Microphone`

`Microphone` is similar to the `Audio` struct, the only difference being that it reads audio from the microphone. The `stream` parameter is used to specify the microphone stream index, which will differ depending on the platform. For Windows (`dshow`) and MacOS (`avfoundation`), find the stream index by entering the following command

```
ffmpeg -f [dshow | avfoundation] -list_devices true -i dummy
```

and selecting the desired stream. For linux, see [this page](https://trac.ffmpeg.org/wiki/Capture/PulseAudio) on the FFmpeg Wiki.

Additionally, an `options` parameter may be passed to specify the format, sampling rate and audio channels the microphone should record at. Any other options are ignored.

```go
aio.NewMicrophone(stream int, options *aio.Options) (*aio.Microphone, error)

Name() string
SampleRate() int
Channels() int
Format() string
BitsPerSample() int
Buffer() []byte
Samples() interface{}
SetBuffer(buffer []byte) error

Read() bool
Close()
```

## `Player`

`Player` is used to play audio from a buffer of audio samples.

```go
aio.NewPlayer(channels, samplerate int, format string) (*aio.Player, error)

SampleRate() int
Channels() int
Format() string
Play(samples interface{}) error
Close()
```

## Examples

Copy `input.wav` to `output.mp3`.

```go
audio, _ := aio.NewAudio("input.wav", nil)

options := aio.Options{
	SampleRate: audio.SampleRate(),
	Channels:   audio.Channels(),
	Bitrate:    audio.Bitrate(),
	Format:     audio.Format(),
}

writer, _ := aio.NewAudioWriter("output.mp3", &options)
defer writer.Close()

for audio.Read() {
	writer.Write(audio.Buffer())
}
```

Capture 10 seconds of audio from the microphone. Audio is recorded at 44100 Hz stereo and is in signed 16 bit format.

```go
micOptions := aio.Options{Format: "s16", Channels: 2, SampleRate: 44100}
mic, _ := aio.NewMicrophone(0, &micOptions)
defer mic.Close()

writerOptions := aio.Options{
	SampleRate: mic.SampleRate(),
	Channels:   mic.Channels(),
	Format:     mic.Format(),
}

writer, _ := aio.NewAudioWriter("output.wav", &writerOptions)
defer writer.Close()

seconds := 0
for mic.Read() && seconds < 10 {
	writer.Write(mic.Buffer())
	seconds++
}
```

Play all audio tracks from `input.mp4` sequentially.

```go
streams, _ := aio.NewAudioStreams("input.mp4", nil)

for _, stream := range streams {
	player, _ := aio.NewPlayer(stream.Channels(), stream.SampleRate(), stream.Format())
	for stream.Read() {
		player.Play(stream.Buffer())
	}
	player.Close()
}
```

Play `input.mp4`.

```go
audio, _ := aio.NewAudio("input.mp4", nil)
player, _ := aio.NewPlayer(audio.Channels(), audio.SampleRate(), audio.Format())
defer player.Close()

for audio.Read() {
	player.Play(audio.Buffer())
}
```

Read `input.wav` and process the audio samples.

```go
audio, _ := aio.NewAudio("input.wav", nil)

for audio.Read() {
	samples := audio.Samples().([]int16)
	for i := range samples {
		// audio processing...
	}
}
```

Combine `sound.wav` and `movie.mp4` into `output.mp4`.

```go
audio, _ := aio.NewAudio("sound.wav", nil)

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

Play Microphone audio. Use default microphone settings for recording.

```go
mic, _ := aio.NewMicrophone(0, nil)
defer mic.Close()

player, _ := aio.NewPlayer(mic.Channels(), mic.SampleRate(), mic.Format())
defer player.Close()

for mic.Read() {
	player.Play(mic.Buffer())
}
```