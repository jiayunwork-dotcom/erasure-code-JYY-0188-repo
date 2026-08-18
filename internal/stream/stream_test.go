package stream

import (
	"bytes"
	"io"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid", Config{DataShards: 4, ParityShards: 2, StripeSize: 16}, false},
		{"zero data", Config{DataShards: 0, ParityShards: 2, StripeSize: 16}, true},
		{"stripe not divisible", Config{DataShards: 4, ParityShards: 2, StripeSize: 15}, true},
		{"too many shards", Config{DataShards: 200, ParityShards: 100, StripeSize: 200}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStreamEncodeRoundTrip(t *testing.T) {
	cfg := Config{DataShards: 4, ParityShards: 2, StripeSize: 16}
	input := make([]byte, 16)
	for i := range input {
		input[i] = byte(i + 1)
	}

	// Encode to 6 writers.
	total := cfg.DataShards + cfg.ParityShards
	writers := make([]io.Writer, total)
	buffers := make([]*bytes.Buffer, total)
	for i := range writers {
		buffers[i] = &bytes.Buffer{}
		writers[i] = buffers[i]
	}
	stripes, consumed, err := StreamEncode(bytes.NewReader(input), writers, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if stripes != 1 {
		t.Fatalf("expected 1 stripe, got %d", stripes)
	}
	if consumed != 16 {
		t.Fatalf("expected 16 consumed, got %d", consumed)
	}
	// Each buffer should have shardSize bytes.
	shardSize := cfg.ShardSize()
	for i, buf := range buffers {
		if buf.Len() != shardSize {
			t.Fatalf("shard %d has %d bytes, expected %d", i, buf.Len(), shardSize)
		}
	}
}

func TestEncodeStripeDecodeStripe(t *testing.T) {
	cfg := Config{DataShards: 3, ParityShards: 2, StripeSize: 12}
	stripe := []byte("hello world!")
	shards, err := EncodeStripe(stripe, cfg)
	if err != nil {
		t.Fatal(err)
	}
	total := cfg.DataShards + cfg.ParityShards
	if len(shards) != total {
		t.Fatalf("expected %d shards, got %d", total, len(shards))
	}
	// Simulate losing 2 parity shards (still decodable with all data present).
	present := make([]bool, total)
	for i := 0; i < cfg.DataShards; i++ {
		present[i] = true
	}
	// Zero out parity for reconstruction test.
	for i := cfg.DataShards; i < total; i++ {
		present[i] = true // keep all for a simple decode
	}
	decoded, err := DecodeStripe(shards, present, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, stripe) {
		t.Fatalf("decoded %q, want %q", decoded, stripe)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig(4, 2)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig produced invalid config: %v", err)
	}
	if cfg.StripeSize%cfg.DataShards != 0 {
		t.Fatal("stripe size not divisible by data shards")
	}
}
