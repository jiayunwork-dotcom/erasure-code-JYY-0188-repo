// Package interleave provides interleaved Reed-Solomon encoding. Data is split
// into blocks, transposed (rows become columns), and then each column is
// independently RS-encoded. The interleaving distributes burst errors across
// multiple RS codewords, improving burst-error tolerance.
package interleave

import (
	"errors"

	"erasure-code/internal/codec"
)

// ErrInvalidConfig is returned when interleave parameters are out of range.
var ErrInvalidConfig = errors.New("interleave: invalid configuration")

// ErrDataTooShort is returned when the input data is too short for the given
// interleave depth.
var ErrDataTooShort = errors.New("interleave: data too short")

// ErrInconsistentShards is returned when shard dimensions are inconsistent.
var ErrInconsistentShards = errors.New("interleave: inconsistent shard dimensions")

// Config describes the interleaving parameters.
type Config struct {
	DataShards   int // RS data shard count per column
	ParityShards int // RS parity shard count per column
	Depth        int // interleave depth (number of columns)
}

// Validate checks that the configuration has sensible values.
func (c Config) Validate() error {
	if c.DataShards <= 0 || c.ParityShards <= 0 || c.Depth <= 0 {
		return ErrInvalidConfig
	}
	if c.DataShards+c.ParityShards > 255 {
		return ErrInvalidConfig
	}
	return nil
}

// TotalShards returns the total number of shards per column.
func (c Config) TotalShards() int { return c.DataShards + c.ParityShards }

// BlockSize returns the number of data bytes needed per encode unit.
func (c Config) BlockSize() int { return c.DataShards * c.Depth }

// Encode performs interleaved RS encoding on data. The data is arranged as a
// DataShards x Depth matrix (row-major), then each of the Depth columns is
// independently RS-encoded, producing ParityShards additional rows. The result
// is a (DataShards+ParityShards) x Depth matrix returned in row-major order.
func Encode(data []byte, cfg Config) ([][]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	blockSize := cfg.BlockSize()
	if len(data) < blockSize {
		return nil, ErrDataTooShort
	}
	// Arrange data into DataShards rows of Depth bytes each.
	rows := make([][]byte, cfg.DataShards)
	for r := 0; r < cfg.DataShards; r++ {
		rows[r] = make([]byte, cfg.Depth)
		copy(rows[r], data[r*cfg.Depth:(r+1)*cfg.Depth])
	}
	// For each column, collect the DataShards bytes and encode a column-vector.
	parityRows := make([][]byte, cfg.ParityShards)
	for p := 0; p < cfg.ParityShards; p++ {
		parityRows[p] = make([]byte, cfg.Depth)
	}
	for col := 0; col < cfg.Depth; col++ {
		colData := make([]byte, cfg.DataShards)
		for r := 0; r < cfg.DataShards; r++ {
			colData[r] = rows[r][col]
		}
		// Encode this column: each byte is a 1-byte "shard" in RS terms.
		encoded, err := codec.Encode(colData, cfg.DataShards, cfg.ParityShards)
		if err != nil {
			return nil, err
		}
		for p := 0; p < cfg.ParityShards; p++ {
			parityRows[p][col] = encoded[cfg.DataShards+p][0]
		}
	}
	// Assemble output: data rows + parity rows.
	total := cfg.TotalShards()
	out := make([][]byte, total)
	for r := 0; r < cfg.DataShards; r++ {
		out[r] = rows[r]
	}
	for p := 0; p < cfg.ParityShards; p++ {
		out[cfg.DataShards+p] = parityRows[p]
	}
	return out, nil
}

// Reconstruct recovers missing rows in the interleaved matrix. Each column is
// independently reconstructed. present marks which rows are available.
func Reconstruct(shards [][]byte, present []bool, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	total := cfg.TotalShards()
	if len(shards) != total || len(present) != total {
		return ErrInconsistentShards
	}
	// Determine depth from present shards.
	depth := 0
	for i := range shards {
		if present[i] {
			depth = len(shards[i])
			break
		}
	}
	if depth == 0 {
		return ErrInconsistentShards
	}
	// Allocate missing shards.
	for i := range shards {
		if !present[i] {
			shards[i] = make([]byte, depth)
		}
	}
	// Reconstruct column by column.
	for col := 0; col < depth; col++ {
		colShards := make([][]byte, total)
		colPresent := make([]bool, total)
		for r := 0; r < total; r++ {
			colShards[r] = []byte{shards[r][col]}
			colPresent[r] = present[r]
		}
		if err := codec.ReconstructInPlace(colShards, colPresent, cfg.DataShards); err != nil {
			return err
		}
		for r := 0; r < total; r++ {
			shards[r][col] = colShards[r][0]
		}
	}
	return nil
}
