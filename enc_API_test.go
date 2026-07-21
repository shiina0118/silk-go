package silk

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"testing"
)

func TestEncode24KHz(t *testing.T) {
	for _, test := range []struct {
		name                  string
		maxInternalSampleRate int32
		bitRate               int32
		wantSHA256            string
	}{
		{name: "24khz-internal", maxInternalSampleRate: 24000, bitRate: 24000, wantSHA256: "f35bd87820e3d21f833419535e995474fa2b3706aa93186fcfa3e42142ea96bb"},
		{name: "16khz-internal", maxInternalSampleRate: 16000, bitRate: 16000, wantSHA256: "edfcb1d53e644657249b66e6c55575adff7e3cbf8748acc97245ed839a16a35b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			pcm := testPCM(t, 24000, 5)
			var encoded bytes.Buffer
			err := Encode(&EncoderOption{
				SampleRate:            24000,
				MaxInternalSampleRate: test.maxInternalSampleRate,
				PacketSize:            480,
				BitRate:               test.bitRate,
				Complexity:            2,
			}, bytes.NewReader(pcm), &encoded)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if !bytes.HasPrefix(encoded.Bytes(), magicV3) {
				t.Fatal("Encode() did not write a SILK V3 header")
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(encoded.Bytes())); got != test.wantSHA256 {
				t.Fatalf("encoded SHA-256 = %s, want %s", got, test.wantSHA256)
			}
			if got, want := decodedSampleCount(t, encoded.Bytes(), 24000), len(pcm)/2; got != want {
				t.Fatalf("decoded sample count = %d, want %d", got, want)
			}
		})
	}
}

func BenchmarkEncode24KHz(b *testing.B) {
	pcm := testPCM(b, 24000, 30)
	options := &EncoderOption{
		SampleRate:            24000,
		MaxInternalSampleRate: 24000,
		PacketSize:            480,
		BitRate:               24000,
		Complexity:            2,
	}
	b.SetBytes(int64(len(pcm)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		var encoded bytes.Buffer
		if err := Encode(options, bytes.NewReader(pcm), &encoded); err != nil {
			b.Fatal(err)
		}
	}
}

func decodedSampleCount(t *testing.T, encoded []byte, sampleRate int) int {
	t.Helper()
	reader := bytes.NewReader(encoded)
	header := make([]byte, len(magicV3))
	if _, err := io.ReadFull(reader, header); err != nil {
		t.Fatalf("read SILK header: %v", err)
	}
	if !bytes.Equal(header, magicV3) {
		t.Fatal("invalid SILK V3 header")
	}

	decoder := NewDecoder(sampleRate)
	total := 0
	for reader.Len() > 0 {
		var payloadSize uint16
		if err := binary.Read(reader, binary.LittleEndian, &payloadSize); err != nil {
			t.Fatalf("read payload size: %v", err)
		}
		payload := make([]byte, payloadSize)
		if _, err := io.ReadFull(reader, payload); err != nil {
			t.Fatalf("read payload: %v", err)
		}
		decoded, err := decoder.DecodePayload(payload, false)
		if err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		total += len(decoded.Data)
	}
	return total
}

func testPCM(t testing.TB, sampleRate, durationSeconds int) []byte {
	t.Helper()
	pcm := make([]byte, sampleRate*durationSeconds*2)
	for index := 0; index < sampleRate*durationSeconds; index++ {
		sample := int16(math.Sin(2*math.Pi*440*float64(index)/float64(sampleRate)) * 12000)
		binary.LittleEndian.PutUint16(pcm[index*2:], uint16(sample))
	}
	return pcm
}
