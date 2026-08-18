package xor

import (
	"bytes"
	"testing"
)

func TestBytesBasic(t *testing.T) {
	dst := []byte{0x01, 0x02, 0x03, 0x04}
	src := []byte{0x10, 0x20, 0x30, 0x40}
	if err := Bytes(dst, src); err != nil {
		t.Fatal(err)
	}
	want := []byte{0x11, 0x22, 0x33, 0x44}
	if !bytes.Equal(dst, want) {
		t.Fatalf("got %x, want %x", dst, want)
	}
}

func TestBytesLengthMismatch(t *testing.T) {
	dst := make([]byte, 3)
	src := make([]byte, 5)
	if err := Bytes(dst, src); err != ErrLengthMismatch {
		t.Fatalf("expected ErrLengthMismatch, got %v", err)
	}
}

func TestBytesToThreeWay(t *testing.T) {
	a := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}
	b := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	dst := make([]byte, 8)
	if err := BytesTo(dst, a, b); err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if dst[i] != a[i]^b[i] {
			t.Fatalf("mismatch at %d: got %02x, want %02x", i, dst[i], a[i]^b[i])
		}
	}
}

func TestMulti(t *testing.T) {
	s1 := []byte{0x01, 0x02}
	s2 := []byte{0x04, 0x08}
	s3 := []byte{0x10, 0x20}
	dst := make([]byte, 2)
	if err := Multi(dst, s1, s2, s3); err != nil {
		t.Fatal(err)
	}
	want := []byte{0x01 ^ 0x04 ^ 0x10, 0x02 ^ 0x08 ^ 0x20}
	if !bytes.Equal(dst, want) {
		t.Fatalf("got %x, want %x", dst, want)
	}
}

func TestHammingDistance(t *testing.T) {
	a := []byte{0xFF, 0x00}
	b := []byte{0x00, 0x00}
	dist, err := HammingDistance(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if dist != 8 {
		t.Fatalf("expected hamming distance 8, got %d", dist)
	}
}

func TestFoldAndParity(t *testing.T) {
	rows := [][]byte{
		{0x01, 0x02, 0x03},
		{0x04, 0x05, 0x06},
		{0x07, 0x08, 0x09},
	}
	parity, err := Parity(rows)
	if err != nil {
		t.Fatal(err)
	}
	// parity = rows[0] ^ rows[1] ^ rows[2]
	for i := range parity {
		want := rows[0][i] ^ rows[1][i] ^ rows[2][i]
		if parity[i] != want {
			t.Fatalf("parity[%d] = %02x, want %02x", i, parity[i], want)
		}
	}
}

func TestRepairXOR(t *testing.T) {
	rows := [][]byte{
		{0xAA, 0xBB},
		{0xCC, 0xDD},
		{0xEE, 0xFF},
	}
	parity, _ := Parity(rows)
	// Lose row 1, recover it.
	remaining := [][]byte{rows[0], rows[2]}
	recovered, err := RepairXOR(parity, remaining)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recovered, rows[1]) {
		t.Fatalf("recovered %x, want %x", recovered, rows[1])
	}
}
