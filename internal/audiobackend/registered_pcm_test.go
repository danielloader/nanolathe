package audiobackend

import (
	"bytes"
	"io"
	"testing"

	retailaudio "github.com/nanolathe-gg/nanolathe/internal/audio"
)

func TestPanReaderMatchesWholeConversionAcrossShortReads(t *testing.T) {
	sample := &retailaudio.Sample{Channels: 1, SampleRate: 11025, BitsPerSample: 8, Data: []byte{128, 255, 0}}
	canonical := sample.RegisteredPCM(44100)
	reader := newPanReader(canonical, -0.25)
	var got []byte
	for _, size := range []int{1, 3, 5, 2, 7, 4} {
		buf := make([]byte, size)
		n, err := reader.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, remaining...)
	want := retailaudio.ConvertSample(sample, 1, -0.25, 44100)
	if !bytes.Equal(got, want) {
		t.Fatal("short reads changed the panned PCM byte stream")
	}
}

// Device reads mix whole frames written directly with frames a short or
// unaligned read splits; every mix, wrapped or not, must keep the stream.
func TestPanReaderMixedReadSizesMatchWholeConversion(t *testing.T) {
	sample := oneSecondMono8()
	for _, pan := range []float64{-0.75, 0, 0.4} {
		want := retailaudio.ConvertSample(sample, 1, pan, 44100)
		reader := newPanReader(sample.RegisteredPCM(44100), pan)
		reader.SetLoop(true)
		var got []byte
		for i, size := 0, 0; len(got) < 3*len(want); i++ {
			size = []int{4096, 3, 8, 13, 70001, 1, 7, 8192}[i%8]
			buf := make([]byte, size)
			n, err := reader.Read(buf)
			if err != nil || n != size {
				t.Fatalf("pan %v read %d = %d, %v", pan, size, n, err)
			}
			got = append(got, buf...)
		}
		for start := 0; start < len(got); start += len(want) {
			end := min(start+len(want), len(got))
			if !bytes.Equal(got[start:end], want[:end-start]) {
				t.Fatalf("pan %v: mixed reads changed the looped PCM stream at byte %d", pan, start)
			}
		}
	}
}

func TestRegisteredPlaysKeepPanAndReplacementIndependent(t *testing.T) {
	b := NewWithRate(44100)
	var played [][]byte
	b.createPlayer = func(source io.Reader) (outputPlayer, error) {
		data, err := io.ReadAll(source)
		if err != nil {
			return nil, err
		}
		played = append(played, data)
		return &observedPlayer{}, nil
	}
	first := &retailaudio.Sample{Channels: 1, SampleRate: 11025, BitsPerSample: 8, Data: []byte{255, 255}}
	if err := b.PlayRegisteredSample(first, 1, -1); err != nil {
		t.Fatal(err)
	}
	if err := b.PlayRegisteredSample(first, 1, 1); err != nil {
		t.Fatal(err)
	}
	replacement := &retailaudio.Sample{Channels: 1, SampleRate: 11025, BitsPerSample: 8, Data: []byte{0, 0}}
	if err := b.PlayRegisteredSample(replacement, 1, 0); err != nil {
		t.Fatal(err)
	}
	if len(played) != 3 {
		t.Fatalf("plays=%d, want 3", len(played))
	}
	if bytes.Equal(played[0], played[1]) {
		t.Fatal("simultaneous registered plays shared pan state")
	}
	if bytes.Equal(played[0], played[2]) || bytes.Equal(played[1], played[2]) {
		t.Fatal("replacement sample reused prior canonical PCM")
	}
}

func BenchmarkRegisteredPCMAdmission(b *testing.B) {
	sample := oneSecondMono8()
	backend := NewWithRate(44100)
	backend.createPlayer = func(io.Reader) (outputPlayer, error) { return &observedPlayer{}, nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.PlayRegisteredSample(sample, 1, 0.25)
	}
}

func BenchmarkConvertedPCMAdmission(b *testing.B) {
	sample := oneSecondMono8()
	backend := NewWithRate(44100)
	backend.createPlayer = func(io.Reader) (outputPlayer, error) { return &observedPlayer{}, nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.PlaySample(sample, 1, 0.25)
	}
}

func BenchmarkRegisteredPCMReader(b *testing.B) {
	sample := oneSecondMono8()
	canonical := sample.RegisteredPCM(44100)
	buffer := make([]byte, 4096)
	b.SetBytes(int64(len(canonical)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := newPanReader(canonical, 0.25)
		for {
			_, err := reader.Read(buffer)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkRegisteredPCMPlaybackReader(b *testing.B) {
	sample := oneSecondMono8()
	backend := NewWithRate(44100)
	backend.createPlayer = func(source io.Reader) (outputPlayer, error) {
		_, err := io.Copy(io.Discard, source)
		return &observedPlayer{}, err
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.PlayRegisteredSample(sample, 1, 0.25)
	}
}

func BenchmarkConvertedPCMPlaybackReader(b *testing.B) {
	sample := oneSecondMono8()
	backend := NewWithRate(44100)
	backend.createPlayer = func(source io.Reader) (outputPlayer, error) {
		_, err := io.Copy(io.Discard, source)
		return &observedPlayer{}, err
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.PlaySample(sample, 1, 0.25)
	}
}

func BenchmarkConvertSample(b *testing.B) {
	sample := oneSecondMono8()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retailaudio.ConvertSample(sample, 1, 0.25, 44100)
	}
}

func oneSecondMono8() *retailaudio.Sample {
	data := make([]byte, 11025)
	for i := range data {
		data[i] = byte(i)
	}
	return &retailaudio.Sample{Channels: 1, SampleRate: 11025, BitsPerSample: 8, Data: data}
}
