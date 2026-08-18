// Package stream provides streaming Reed-Solomon encoding and decoding for
// large payloads that do not fit comfortably in memory. Data is processed in
// fixed-size stripes (blocks), each independently encoded into data+parity
// shards. This enables parallel I/O and bounded memory usage.
package stream

import (
	"errors"
	"io"

	"erasure-code/internal/codec"
)

// DefaultStripeSize is the default number of bytes per stripe (before splitting
// into shards). 64 KiB is a practical balance between memory and IO granularity.
const DefaultStripeSize = 64 * 1024

// ErrInvalidConfig is returned when stream parameters are invalid.
var ErrInvalidConfig = errors.New("stream: invalid configuration")

// ErrShortWrite is returned when a writer fails to consume all bytes.
var ErrShortWrite = errors.New("stream: short write")

// Config holds the streaming encoder/decoder parameters.
type Config struct {
	DataShards   int // number of data shards per stripe
	ParityShards int // number of parity shards per stripe
	StripeSize   int // total bytes per stripe (split across data shards)
}

// Validate checks that a Config has sensible values.
func (c Config) Validate() error {
	if c.DataShards <= 0 || c.ParityShards <= 0 {
		return ErrInvalidConfig
	}
	if c.DataShards+c.ParityShards > 255 {
		return ErrInvalidConfig
	}
	if c.StripeSize <= 0 {
		return ErrInvalidConfig
	}
	if c.StripeSize%c.DataShards != 0 {
		return ErrInvalidConfig
	}
	return nil
}

// ShardSize returns the per-shard size for one stripe.
func (c Config) ShardSize() int {
	return c.StripeSize / c.DataShards
}

// DefaultConfig returns a Config with sensible defaults for the given shard
// counts.
func DefaultConfig(data, parity int) Config {
	stripe := DefaultStripeSize
	// Round stripe up to be divisible by data shards.
	if stripe%data != 0 {
		stripe += data - (stripe % data)
	}
	return Config{
		DataShards:   data,
		ParityShards: parity,
		StripeSize:   stripe,
	}
}

// EncodeStripe encodes a single stripe of raw data into data+parity shards.
// The input must be exactly config.StripeSize bytes.
func EncodeStripe(stripe []byte, cfg Config) ([][]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if len(stripe) != cfg.StripeSize {
		return nil, errors.New("stream: stripe size mismatch")
	}
	return codec.Encode(stripe, cfg.DataShards, cfg.ParityShards)
}

// DecodeStripe reconstructs a stripe from available shards. The present slice
// marks which shards are available. Missing shards are rebuilt and the original
// stripe data (sans parity) is returned.
func DecodeStripe(shards [][]byte, present []bool, cfg Config) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := codec.ReconstructInPlace(shards, present, cfg.DataShards); err != nil {
		return nil, err
	}
	// Reassemble from data shards.
	out := make([]byte, 0, cfg.StripeSize)
	for i := 0; i < cfg.DataShards; i++ {
		out = append(out, shards[i]...)
	}
	return out, nil
}

// StreamEncode reads from r in stripe-sized chunks, encodes each, and writes
// shard data to the shard writers. The writers slice must have length
// DataShards+ParityShards. Returns the number of full stripes written and the
// number of data bytes consumed.
func StreamEncode(r io.Reader, writers []io.Writer, cfg Config) (stripes int, consumed int64, err error) {
	if err = cfg.Validate(); err != nil {
		return 0, 0, err
	}
	total := cfg.DataShards + cfg.ParityShards
	if len(writers) != total {
		return 0, 0, errors.New("stream: writers count must equal data+parity")
	}
	buf := make([]byte, cfg.StripeSize)
	for {
		n, rerr := io.ReadFull(r, buf)
		if n == 0 {
			break
		}
		// Zero-pad a short final stripe.
		if n < cfg.StripeSize {
			for i := n; i < cfg.StripeSize; i++ {
				buf[i] = 0
			}
		}
		shards, encErr := EncodeStripe(buf, cfg)
		if encErr != nil {
			return stripes, consumed, encErr
		}
		for i, w := range writers {
			if _, werr := w.Write(shards[i]); werr != nil {
				return stripes, consumed, werr
			}
		}
		stripes++
		consumed += int64(n)
		if rerr != nil {
			break
		}
	}
	return stripes, consumed, nil
}
