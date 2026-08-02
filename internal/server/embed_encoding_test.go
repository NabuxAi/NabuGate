package server

import (
	"encoding/base64"
	"encoding/binary"
	"math"
	"testing"
)

// The gateway used to drop `encoding_format` entirely and always answer with a
// JSON array of floats.
//
// That is not a cosmetic difference. The official OpenAI SDK sends
// `encoding_format: "base64"` by DEFAULT, so most callers ask for it without
// ever deciding to, and a client that asks for base64 then decodes whatever it
// receives as packed float32 bytes. Given a JSON array it produces a vector a
// quarter of the expected length — 384 floats where 1536 were promised —
// without raising anything. The vector store rejects the write, the ingest
// reports success, and the datastore stays empty.
//
// So these tests are about a decoded vector matching the one that went in, not
// about the string being well-formed.

// decodeBase64Embedding is what a client does with the response: little-endian
// float32, exactly as OpenAI encodes it.
func decodeBase64Embedding(t *testing.T, encoded string) []float32 {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("response was not valid base64: %v", err)
	}
	if len(raw)%4 != 0 {
		t.Fatalf("base64 payload is %d bytes, not a whole number of float32s", len(raw))
	}
	out := make([]float32, len(raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return out
}

func TestFloatFormatIsUnchanged(t *testing.T) {
	vec := []float64{0.5, -0.25, 1}

	for _, format := range []string{"", "float"} {
		got, ok := encodeEmbedding(vec, format).([]float64)
		if !ok {
			t.Fatalf("format %q did not return a float slice", format)
		}
		if len(got) != len(vec) {
			t.Fatalf("format %q returned %d values, want %d", format, len(got), len(vec))
		}
	}
}

func TestBase64RoundTripsToTheSameVector(t *testing.T) {
	vec := []float64{0.5, -0.25, 0, 1, -1, 0.125}

	encoded, ok := encodeEmbedding(vec, "base64").(string)
	if !ok {
		t.Fatal("base64 format did not return a string")
	}

	got := decodeBase64Embedding(t, encoded)

	if len(got) != len(vec) {
		t.Fatalf("decoded %d values, want %d — this is the bug that halves or quarters a vector",
			len(got), len(vec))
	}
	for i := range vec {
		if got[i] != float32(vec[i]) {
			t.Errorf("value %d decoded as %v, want %v", i, got[i], float32(vec[i]))
		}
	}
}

// The length is the property that breaks a vector store, so it is worth pinning
// at the width the ecosystem actually uses.
func TestBase64PreservesA1536WideVector(t *testing.T) {
	vec := make([]float64, 1536)
	for i := range vec {
		vec[i] = float64(i%7) / 7
	}

	encoded := encodeEmbedding(vec, "base64").(string)
	got := decodeBase64Embedding(t, encoded)

	if len(got) != 1536 {
		t.Fatalf("a 1536-wide vector decoded to %d — Qdrant would refuse this write", len(got))
	}
}

func TestBase64HandlesAnEmptyVector(t *testing.T) {
	encoded, ok := encodeEmbedding([]float64{}, "base64").(string)
	if !ok {
		t.Fatal("base64 format did not return a string for an empty vector")
	}
	if encoded != "" {
		t.Errorf("an empty vector encoded to %q, want the empty string", encoded)
	}
}

// Anything other than the two known formats must be refused by the handler
// before it reaches here; this pins the encoder's own fallback so an unexpected
// value can never silently produce base64 for a caller expecting floats.
func TestUnknownFormatFallsBackToFloats(t *testing.T) {
	if _, ok := encodeEmbedding([]float64{1, 2}, "hex").([]float64); !ok {
		t.Error("an unrecognised format did not fall back to floats")
	}
}
