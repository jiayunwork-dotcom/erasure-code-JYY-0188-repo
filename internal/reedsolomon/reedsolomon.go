// Package reedsolomon is a facade over the galois and codec packages. It offers
// a small, predictable API for splitting a payload into shards, encoding parity,
// reconstructing missing shards, and verifying integrity. All operations are
// deterministic for a given input and shard configuration.
package reedsolomon

import (
	"bytes"
	"errors"

	"erasure-code/internal/codec"
	"erasure-code/internal/galois"
)

// ErrNoShards is returned when an empty shard slice is supplied.
var ErrNoShards = errors.New("reedsolomon: no shards provided")

// ErrInvalidShardCount is returned for non-positive or inconsistent shard counts.
var ErrInvalidShardCount = errors.New("reedsolomon: invalid shard count")

// ErrPresentMismatch is returned when the present slice length does not match the
// number of shards.
var ErrPresentMismatch = errors.New("reedsolomon: present length does not match shards")

// ErrTooFewShards is returned when fewer than dataShards shards are available for
// reconstruction.
var ErrTooFewShards = errors.New("reedsolomon: too few shards to reconstruct")

// ErrSizeMismatch is returned when shards have inconsistent lengths.
var ErrSizeMismatch = errors.New("reedsolomon: shard size mismatch")

// ErrUnrecoverable is returned when the available shards do not form an invertible
// submatrix (this should not happen for valid configurations unless too few
// shards are present).
var ErrUnrecoverable = errors.New("reedsolomon: shards are not recoverable")

// maxTotalShards mirrors codec's field-size bound.
const maxTotalShards = 255

// Split divides data into dataShards equal-length data shards and allocates zero
// parity shards, returning a slice of length dataShards+parityShards. The data
// shards carry the (padded) payload; parity shards are left zeroed for the caller
// (typically Encode) to fill.
func Split(data []byte, dataShards, parityShards int) ([][]byte, error) {
	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvalidShardCount
	}
	total := dataShards + parityShards
	if total > maxTotalShards {
		return nil, ErrInvalidShardCount
	}
	size := 0
	if len(data) > 0 {
		size = (len(data) + dataShards - 1) / dataShards
	}
	padded := make([]byte, dataShards*size)
	copy(padded, data)
	shards := make([][]byte, total)
	for i := 0; i < dataShards; i++ {
		shards[i] = padded[i*size : (i+1)*size]
	}
	for i := dataShards; i < total; i++ {
		shards[i] = make([]byte, size)
	}
	return shards, nil
}

// Encode splits data into dataShards data shards and computes parityShards parity
// shards using a Vandermonde-based Reed-Solomon code. It returns all shards, each
// of equal length.
func Encode(data []byte, dataShards, parityShards int) ([][]byte, error) {
	return codec.Encode(data, dataShards, parityShards)
}

// Reconstruct recovers any missing shards in-place. shards has length
// dataShards+parityShards; present marks the shards that are available. At least
// dataShards shards must be present. Missing shards are recomputed from the
// available ones. It returns ErrTooFewShards when too few are present.
func Reconstruct(shards [][]byte, present []bool, dataShards int) error {
	total := len(shards)
	if total == 0 {
		return ErrNoShards
	}
	parityShards := total - dataShards
	if dataShards <= 0 || parityShards <= 0 {
		return ErrInvalidShardCount
	}
	if len(present) != total {
		return ErrPresentMismatch
	}
	count := 0
	for _, p := range present {
		if p {
			count++
		}
	}
	if count < dataShards {
		return ErrTooFewShards
	}
	if total > maxTotalShards {
		return ErrInvalidShardCount
	}
	// Determine the shard size from a present shard and validate that all present
	// shards share that size. Missing (nil) shards are skipped here and rebuilt.
	size := -1
	for i := 0; i < total; i++ {
		if present[i] {
			size = len(shards[i])
			break
		}
	}
	if size < 0 {
		return ErrNoShards
	}
	for i := 0; i < total; i++ {
		if present[i] && len(shards[i]) != size {
			return ErrSizeMismatch
		}
	}

	C, err := codec.BuildCodeMatrix(dataShards, parityShards)
	if err != nil {
		return err
	}

	// Build the dataShards x dataShards submatrix from the available rows.
	sub := make([][]byte, dataShards)
	chosen := make([][]byte, dataShards)
	pick := 0
	for i := 0; i < total && pick < dataShards; i++ {
		if present[i] {
			sub[pick] = C[i]
			chosen[pick] = shards[i]
			pick++
		}
	}
	if pick != dataShards {
		return ErrTooFewShards
	}
	invSub, err := codec.Invert(sub)
	if err != nil {
		return ErrUnrecoverable
	}

	// Recover the data vector D: D[c] = sum_r invSub[c][r] * chosen[r].
	data := make([][]byte, dataShards)
	for c := 0; c < dataShards; c++ {
		data[c] = make([]byte, size)
		for r := 0; r < dataShards; r++ {
			galois.MulSlice(invSub[c][r], chosen[r], data[c])
		}
	}

	// Fill missing shards i using row C[i] of the code matrix.
	for i := 0; i < total; i++ {
		if !present[i] {
			rebuilt := make([]byte, size)
			for c := 0; c < dataShards; c++ {
				galois.MulSlice(C[i][c], data[c], rebuilt)
			}
			shards[i] = rebuilt
		}
	}
	return nil
}

// Verify checks that the parity shards are consistent with the data shards. It
// returns true when every parity shard equals the value recomputed from the data
// shards. All data shards must be present.
func Verify(shards [][]byte, dataShards int) (bool, error) {
	total := len(shards)
	parityShards := total - dataShards
	if dataShards <= 0 || parityShards <= 0 {
		return false, ErrInvalidShardCount
	}
	if total > maxTotalShards {
		return false, ErrInvalidShardCount
	}
	if total == 0 {
		return false, ErrNoShards
	}
	size := len(shards[0])
	for i := 1; i < total; i++ {
		if len(shards[i]) != size {
			return false, ErrSizeMismatch
		}
	}
	C, err := codec.BuildCodeMatrix(dataShards, parityShards)
	if err != nil {
		return false, err
	}
	for i := dataShards; i < total; i++ {
		computed := make([]byte, size)
		for c := 0; c < dataShards; c++ {
			galois.MulSlice(C[i][c], shards[c], computed)
		}
		if !galois.Equal(computed, shards[i]) {
			return false, nil
		}
	}
	return true, nil
}

// OriginalData concatenates the first dataShards shards and trims the result to
// originalSize bytes, reversing the padding applied during encoding.
func OriginalData(shards [][]byte, dataShards, originalSize int) ([]byte, error) {
	if dataShards <= 0 || dataShards > len(shards) {
		return nil, ErrInvalidShardCount
	}
	var buf bytes.Buffer
	for i := 0; i < dataShards; i++ {
		buf.Write(shards[i])
	}
	out := buf.Bytes()
	if originalSize < 0 || originalSize > len(out) {
		return nil, errors.New("reedsolomon: original size out of range")
	}
	return out[:originalSize], nil
}
