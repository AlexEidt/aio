package aio

type Options struct {
	Stream     int    // Audio Stream Index to use.
	SampleRate int    // Sample rate in Hz.
	Channels   int    // Number of channels.
	Bitrate    int    // Bitrate.
	Format     string // Format of audio.
	Codec      string // Audio Codec.
	Video      string // Video file to use.
}
