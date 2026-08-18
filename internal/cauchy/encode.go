package cauchy

import (
	"erasure-code/internal/galois"
)

// Encode splits data into dataShards data shards and computes parityShards
// additional parity shards using a Cauchy matrix. The returned slice contains
// dataShards + parityShards equal-length shards.
func Encode(data []byte, dataShards, parityShards int) ([][]byte, error) {
	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvalidParams
	}
	if dataShards+parityShards > maxHalfField {
		return nil, ErrTooManyShards
	}
	if len(data) == 0 {
		return nil, ErrInvalidParams
	}
	size := (len(data) + dataShards - 1) / dataShards
	padded := make([]byte, dataShards*size)
	copy(padded, data)

	shards := make([][]byte, dataShards+parityShards)
	for i := 0; i < dataShards; i++ {
		shards[i] = padded[i*size : (i+1)*size]
	}

	cm, err := CauchyMatrix(dataShards, parityShards)
	if err != nil {
		return nil, err
	}
	for p := 0; p < parityShards; p++ {
		parity := make([]byte, size)
		for c := 0; c < dataShards; c++ {
			galois.MulSlice(cm[p][c], shards[c], parity)
		}
		shards[dataShards+p] = parity
	}
	return shards, nil
}

// Reconstruct recovers any missing shards in-place using the Cauchy code matrix.
// shards has length dataShards+parityShards; present marks which are available.
// At least dataShards shards must be present.
func Reconstruct(shards [][]byte, present []bool, dataShards int) error {
	total := len(shards)
	parityShards := total - dataShards
	if dataShards <= 0 || parityShards <= 0 {
		return ErrInvalidParams
	}
	if len(present) != total {
		return ErrShardSizeMismatch
	}
	count := 0
	for _, p := range present {
		if p {
			count++
		}
	}
	if count < dataShards {
		return ErrNotEnoughShards
	}

	// Find the shard size from available shards.
	size := 0
	for i := range shards {
		if present[i] {
			size = len(shards[i])
			break
		}
	}
	if size == 0 {
		return ErrNotEnoughShards
	}

	full, err := FullMatrix(dataShards, parityShards)
	if err != nil {
		return err
	}

	// Pick dataShards available rows.
	availRows := make([]int, 0, dataShards)
	for i := 0; i < total && len(availRows) < dataShards; i++ {
		if present[i] {
			availRows = append(availRows, i)
		}
	}

	sub := SubMatrix(full, availRows)
	invSub, err := Invert(sub)
	if err != nil {
		return err
	}

	// Recover data shards.
	dataVec := make([][]byte, dataShards)
	for c := 0; c < dataShards; c++ {
		dataVec[c] = make([]byte, size)
		for r := 0; r < dataShards; r++ {
			galois.MulSlice(invSub[c][r], shards[availRows[r]], dataVec[c])
		}
	}

	// Fill missing shards.
	for i := 0; i < total; i++ {
		if !present[i] {
			rebuilt := make([]byte, size)
			row := full[i]
			for c := 0; c < dataShards; c++ {
				galois.MulSlice(row[c], dataVec[c], rebuilt)
			}
			shards[i] = rebuilt
		}
	}
	return nil
}

// Verify checks that the parity shards are consistent with the data shards
// according to the Cauchy encoding matrix. Returns true if all parity shards
// match the expected values.
func Verify(shards [][]byte, dataShards int) (bool, error) {
	total := len(shards)
	parityShards := total - dataShards
	if dataShards <= 0 || parityShards <= 0 {
		return false, ErrInvalidParams
	}
	if total > maxHalfField {
		return false, ErrTooManyShards
	}
	size := len(shards[0])
	for _, s := range shards {
		if len(s) != size {
			return false, ErrShardSizeMismatch
		}
	}
	cm, err := CauchyMatrix(dataShards, parityShards)
	if err != nil {
		return false, err
	}
	for p := 0; p < parityShards; p++ {
		expected := make([]byte, size)
		for c := 0; c < dataShards; c++ {
			galois.MulSlice(cm[p][c], shards[c], expected)
		}
		for i := range expected {
			if expected[i] != shards[dataShards+p][i] {
				return false, nil
			}
		}
	}
	return true, nil
}
