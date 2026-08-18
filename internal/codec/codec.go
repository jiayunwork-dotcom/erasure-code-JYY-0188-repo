// Package codec implements Reed-Solomon encoding using a Vandermonde matrix over
// GF(2^8). Given a payload and the desired number of data and parity shards, it
// produces one shard per output piece, all of equal length (the last shard is
// zero-padded as needed). The encoding is deterministic.
package codec

import (
	"errors"

	"erasure-code/internal/galois"
)

// ErrEmptyData is returned by Encode when the input payload is empty.
var ErrEmptyData = errors.New("codec: empty input data")

// ErrSizeMismatch is returned when shards passed to a helper have inconsistent
// lengths.
var ErrSizeMismatch = errors.New("codec: shard size mismatch")

// shardSize returns the size of each shard needed to hold the payload using the
// given number of data shards. A zero-length payload yields a shard size of 0 so
// that callers can still build empty output shards consistently.
func shardSize(dataLen, dataShards int) int {
	if dataLen == 0 {
		return 0
	}
	return (dataLen + dataShards - 1) / dataShards
}

// padToShards splits data into dataShards equal-length shards, padding the final
// shard with zeros so every shard has length shardSize. It returns the shard
// slice of length dataShards.
func padToShards(data []byte, dataShards, size int) [][]byte {
	shards := make([][]byte, dataShards)
	padded := make([]byte, dataShards*size)
	copy(padded, data)
	for i := 0; i < dataShards; i++ {
		shards[i] = padded[i*size : (i+1)*size]
	}
	return shards
}

// Encode splits data into dataShards data shards and computes parityShards
// additional parity shards. The returned slice has length dataShards+parityShards
// and every shard has equal length. The first dataShards entries are the padded
// data shards; the remainder are the computed parity shards.
func Encode(data []byte, dataShards, parityShards int) ([][]byte, error) {
	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvalidShardCount
	}
	total := dataShards + parityShards
	if total > maxTotalShards {
		return nil, ErrInvalidShardCount
	}
	if len(data) == 0 {
		return nil, ErrEmptyData
	}
	size := shardSize(len(data), dataShards)
	if size == 0 {
		return nil, ErrEmptyData
	}
	m, err := buildEncodingMatrix(dataShards, parityShards)
	if err != nil {
		return nil, err
	}
	shards := padToShards(data, dataShards, size)
	// Compute parity shards: parity[p] = sum_c m[p][c] * dataShards[c].
	for p := 0; p < parityShards; p++ {
		parity := make([]byte, size)
		row := m[p]
		for c := 0; c < dataShards; c++ {
			galois.MulSlice(row[c], shards[c], parity)
		}
		shards = append(shards, parity)
	}
	return shards, nil
}

// ComputeParity overwrites parityShards shards (passed by reference as the
// trailing entries of shards) using the data shards already present in the first
// dataShards entries. It is used after reconstruction to fill missing pieces.
func ComputeParity(shards [][]byte, dataShards, parityShards int) error {
	if len(shards) != dataShards+parityShards {
		return ErrSizeMismatch
	}
	size := 0
	if dataShards > 0 {
		size = len(shards[0])
	}
	m, err := buildEncodingMatrix(dataShards, parityShards)
	if err != nil {
		return err
	}
	for p := 0; p < parityShards; p++ {
		parity := make([]byte, size)
		row := m[p]
		for c := 0; c < dataShards; c++ {
			galois.MulSlice(row[c], shards[c], parity)
		}
		shards[dataShards+p] = parity
	}
	return nil
}

// ReconstructInPlace recovers the missing shards in-place. The shards slice has
// length dataShards+parityShards; present marks which shards are available. At
// least dataShards shards must be present. Missing shards are recomputed from
// the available ones. It returns an error when too few shards are present or the
// recovery submatrix is singular.
func ReconstructInPlace(shards [][]byte, present []bool, dataShards int) error {
	total := len(shards)
	parityShards := total - dataShards
	if dataShards <= 0 || parityShards <= 0 {
		return ErrInvalidShardCount
	}
	if len(present) != total {
		return errors.New("codec: present length does not match shards")
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
	size := -1
	for i := 0; i < total; i++ {
		if present[i] {
			size = len(shards[i])
			break
		}
	}
	if size < 0 {
		return ErrTooFewShards
	}
	for i := 0; i < total; i++ {
		if present[i] && len(shards[i]) != size {
			return ErrSizeMismatch
		}
	}

	C, err := BuildCodeMatrix(dataShards, parityShards)
	if err != nil {
		return err
	}

	// Build the dataShards x dataShards submatrix from the available rows and
	// the corresponding shard vectors.
	sub := newMatrix(dataShards, dataShards)
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
		// Fewer available rows than required (should be caught above, defensive).
		return ErrTooFewShards
	}
	invSub, err := Invert(sub)
	if err != nil {
		return ErrMatrixNotInvertible
	}

	// Recover the data vector D: D[c] = sum_r invSub[c][r] * chosen[r].
	data := make([][]byte, dataShards)
	for c := 0; c < dataShards; c++ {
		data[c] = make([]byte, size)
		for r := 0; r < dataShards; r++ {
			galois.MulSlice(invSub[c][r], chosen[r], data[c])
		}
	}

	// Fill any missing shard i using row C[i] of the code matrix.
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

// ErrTooFewShards is returned by reconstruction when fewer than dataShards
// shards are available.
var ErrTooFewShards = errors.New("codec: too few shards to reconstruct")
