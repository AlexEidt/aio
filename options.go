package aio

type Options struct {
	Stream     int    // Audio Stream Index to use.
	SampleRate int    // Sample rate in Hz.
	Channels   int    // Number of channels.
	Bitrate    int    // Bitrate in bits/s.
	Format     string // Format of audio.
	Codec      string // Audio Codec.
	StreamFile string // File path for extra stream data.
}
