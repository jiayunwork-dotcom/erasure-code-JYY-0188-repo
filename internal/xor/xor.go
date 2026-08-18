// Package xor provides fast bulk XOR operations for byte slices. These are the
// building blocks used by the codec and stream packages when combining shard
// vectors. All functions are pure Go with no assembly or SIMD dependencies.
package xor

import (
	"encoding/binary"
	"errors"
	"unsafe"
)

// ErrLengthMismatch is returned when slices have differing lengths.
var ErrLengthMismatch = errors.New("xor: slice length mismatch")

// wordSize is the size of a machine word in bytes. XOR is applied word-at-a-time
// when slices are large enough and properly aligned.
const wordSize = int(unsafe.Sizeof(uintptr(0)))

// Bytes XORs src into dst (dst[i] ^= src[i]). Both slices must have equal length.
func Bytes(dst, src []byte) error {
	if len(dst) != len(src) {
		return ErrLengthMismatch
	}
	n := len(dst)
	if n == 0 {
		return nil
	}
	// Process in 8-byte chunks when possible.
	i := 0
	for ; i+8 <= n; i += 8 {
		d := binary.LittleEndian.Uint64(dst[i:])
		s := binary.LittleEndian.Uint64(src[i:])
		binary.LittleEndian.PutUint64(dst[i:], d^s)
	}
	for ; i < n; i++ {
		dst[i] ^= src[i]
	}
	return nil
}

// BytesTo writes the XOR of a and b into dst. All three slices must have
// equal length.
func BytesTo(dst, a, b []byte) error {
	if len(a) != len(b) || len(dst) != len(a) {
		return ErrLengthMismatch
	}
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		va := binary.LittleEndian.Uint64(a[i:])
		vb := binary.LittleEndian.Uint64(b[i:])
		binary.LittleEndian.PutUint64(dst[i:], va^vb)
	}
	for ; i < n; i++ {
		dst[i] = a[i] ^ b[i]
	}
	return nil
}

// Multi XORs multiple source slices into dst. All slices must have the same
// length. dst is zeroed before accumulation.
func Multi(dst []byte, srcs ...[]byte) error {
	for i := range dst {
		dst[i] = 0
	}
	for _, src := range srcs {
		if len(src) != len(dst) {
			return ErrLengthMismatch
		}
		if err := Bytes(dst, src); err != nil {
			return err
		}
	}
	return nil
}

// Equal reports whether a XOR b produces all-zero bytes (i.e., a == b bitwise).
func Equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var acc byte
	for i := range a {
		acc |= a[i] ^ b[i]
	}
	return acc == 0
}

// CountOnes returns the number of set bits (Hamming weight) in the XOR of a and
// b. This gives the Hamming distance between the two byte vectors.
func HammingDistance(a, b []byte) (int, error) {
	if len(a) != len(b) {
		return 0, ErrLengthMismatch
	}
	count := 0
	for i := range a {
		v := a[i] ^ b[i]
		// Kernighan bit-count trick.
		for v != 0 {
			v &= v - 1
			count++
		}
	}
	return count, nil
}

// Fold XORs all slices together in a tree-like pattern and returns a single
// slice. Returns nil for an empty input. All slices must have the same length.
func Fold(slices [][]byte) ([]byte, error) {
	if len(slices) == 0 {
		return nil, nil
	}
	n := len(slices[0])
	result := make([]byte, n)
	copy(result, slices[0])
	for i := 1; i < len(slices); i++ {
		if len(slices[i]) != n {
			return nil, ErrLengthMismatch
		}
		if err := Bytes(result, slices[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Parity computes the parity (XOR of all elements) for each byte position
// across the given rows and returns the result vector. This is the simplest
// erasure code: a single XOR parity shard that can reconstruct any one missing
// row when all others are present.
func Parity(rows [][]byte) ([]byte, error) {
	return Fold(rows)
}

// RepairXOR recovers a missing row given the parity and the surviving rows. It
// works only for a single-parity scheme (1 parity shard, at most 1 missing row).
// remaining must be all rows except the missing one.
func RepairXOR(parity []byte, remaining [][]byte) ([]byte, error) {
	if len(parity) == 0 {
		return nil, errors.New("xor: empty parity")
	}
	n := len(parity)
	result := make([]byte, n)
	copy(result, parity)
	for _, r := range remaining {
		if len(r) != n {
			return nil, ErrLengthMismatch
		}
		if err := Bytes(result, r); err != nil {
			return nil, err
		}
	}
	return result, nil
}
