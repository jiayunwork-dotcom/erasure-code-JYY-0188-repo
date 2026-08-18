package codec

import (
	"bytes"
	"math/rand"
	"testing"

	"erasure-code/internal/galois"
)

// randPayload returns a deterministic pseudo-random payload of length n.
func randPayload(n, seed int) []byte {
	rng := rand.New(rand.NewSource(int64(seed)))
	b := make([]byte, n)
	rng.Read(b)
	return b
}

// TestVandermonde checks the structure of the Vandermonde matrix and that any
// square subset of its rows is invertible over GF(2^8).
func TestVandermonde(t *testing.T) {
	const rows, cols = 10, 6
	vm := vandermonde(rows, cols)
	if len(vm) != rows || len(vm[0]) != cols {
		t.Fatalf("vandermonde dimensions = %dx%d, want %dx%d", len(vm), len(vm[0]), rows, cols)
	}
	// V[r][c] == Pow(byte(r+1), c).
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if vm[r][c] != galois.Pow(byte(r+1), c) {
				t.Fatalf("vm[%d][%d]=%d, want Pow(%d,%d)=%d", r, c, vm[r][c], r+1, c, galois.Pow(byte(r+1), c))
			}
		}
	}
	// Every square submatrix (k consecutive rows, k <= cols) must be invertible.
	for k := 1; k <= cols; k++ {
		for start := 0; start+k <= rows; start++ {
			sub := subMatrix(vm, start, 0, k, k)
			if _, err := Invert(sub); err != nil {
				t.Fatalf("vandermonde submatrix rows %d..%d not invertible: %v", start, start+k, err)
			}
		}
	}
}

// TestMatrixInvert verifies inversion on a hand-built matrix and the identity
// A * A^-1 == I over GF(2^8).
func TestMatrixInvert(t *testing.T) {
	// Build A from a Vandermonde so we know it is invertible.
	vm := vandermonde(5, 5)
	a := subMatrix(vm, 0, 0, 5, 5)
	inv, err := Invert(a)
	if err != nil {
		t.Fatalf("invert: %v", err)
	}
	prod, err := multiply(a, inv)
	if err != nil {
		t.Fatalf("multiply: %v", err)
	}
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			want := byte(0)
			if i == j {
				want = 1
			}
			if prod[i][j] != want {
				t.Fatalf("A*A^-1[%d][%d]=%d, want %d", i, j, prod[i][j], want)
			}
		}
	}
	// A clearly singular matrix (two identical rows) must fail.
	singular := newMatrix(3, 3)
	singular[0] = []byte{1, 2, 3}
	singular[1] = []byte{1, 2, 3}
	singular[2] = []byte{4, 5, 6}
	if _, err := Invert(singular); err == nil {
		t.Fatal("expected singular matrix to be non-invertible")
	}
}

// TestEncodeErrors verifies the encoder rejects bad inputs.
func TestEncodeErrors(t *testing.T) {
	cases := []struct {
		name  string
		data  []byte
		k, m  int
		want  error
	}{
		{"empty", nil, 4, 2, ErrEmptyData},
		{"zero data shards", []byte{1, 2, 3}, 0, 2, ErrInvalidShardCount},
		{"zero parity shards", []byte{1, 2, 3}, 4, 0, ErrInvalidShardCount},
		{"too many total", []byte{1, 2, 3}, 200, 100, ErrInvalidShardCount},
	}
	for _, c := range cases {
		_, err := Encode(c.data, c.k, c.m)
		if err != c.want {
			t.Fatalf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

// TestEncodeDeterministic checks that encoding the same input twice yields
// byte-identical shards.
func TestEncodeDeterministic(t *testing.T) {
	data := randPayload(1000, 42)
	a, err := Encode(data, 6, 3)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	b, err := Encode(data, 6, 3)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("shard counts differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			t.Fatalf("shard %d differs between runs", i)
		}
	}
}

// TestEncodeShardSizes verifies every output shard has equal length and that the
// padded payload is preserved.
func TestEncodeShardSizes(t *testing.T) {
	data := []byte("the quick brown fox jumps over the lazy dog")
	shards, err := Encode(data, 5, 4)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(shards) != 9 {
		t.Fatalf("got %d shards, want 9", len(shards))
	}
	size := len(shards[0])
	for i, s := range shards {
		if len(s) != size {
			t.Fatalf("shard %d length %d != %d", i, len(s), size)
		}
	}
	// Concatenating the data shards and trimming to the original length must
	// reproduce the input.
	joined := bytes.Join(shards[:5], nil)
	if !bytes.Equal(joined[:len(data)], data) {
		t.Fatal("padded data shards do not preserve the input")
	}
}

// TestEncodeDecodeRoundtrip encodes several payloads and shard configurations,
// reconstructs from the full set (a no-op recovery), and checks the original
// payload is recoverable.
func TestEncodeDecodeRoundtrip(t *testing.T) {
	configs := []struct {
		k, m int
	}{
		{1, 1}, {2, 1}, {4, 2}, {6, 3}, {10, 4}, {16, 8},
	}
	for _, cfg := range configs {
		for size := 0; size < 5; size++ {
			data := randPayload(1+size*37, cfg.k*100+cfg.m+size)
			shards, err := Encode(data, cfg.k, cfg.m)
			if err != nil {
				t.Fatalf("k=%d m=%d size=%d encode: %v", cfg.k, cfg.m, len(data), err)
			}
			present := make([]bool, len(shards))
			for i := range present {
				present[i] = true
			}
			if err := ReconstructInPlace(shards, present, cfg.k); err != nil {
				t.Fatalf("reconstruct: %v", err)
			}
			joined := bytes.Join(shards[:cfg.k], nil)
			joined = joined[:len(data)]
			if !bytes.Equal(joined, data) {
				t.Fatalf("k=%d m=%d size=%d roundtrip mismatch", cfg.k, cfg.m, len(data))
			}
		}
	}
}

// TestReconstructInPlaceMissing drops a mix of data and parity shards and verifies
// reconstruction recovers them exactly.
func TestReconstructInPlaceMissing(t *testing.T) {
	data := randPayload(777, 11)
	k, m := 8, 4
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Keep a deep copy of the original shards for comparison.
	orig := make([][]byte, len(shards))
	for i := range shards {
		orig[i] = append([]byte(nil), shards[i]...)
	}

	// Patterns of missing shards: drop at most m shards (so >= k remain).
	patterns := [][]int{
		{0}, {k + 0}, {0, 1}, {1, k}, {0, k, k + 1}, {2, 3, k + 2},
	}
	for _, missing := range patterns {
		present := make([]bool, len(shards))
		for i := range present {
			present[i] = true
		}
		for _, idx := range missing {
			present[idx] = false
		}
		if err := ReconstructInPlace(shards, present, k); err != nil {
			t.Fatalf("missing %v reconstruct: %v", missing, err)
		}
		for i := range shards {
			if !bytes.Equal(shards[i], orig[i]) {
				t.Fatalf("missing %v: shard %d not recovered correctly", missing, i)
			}
		}
	}
}

// TestReconstructInPlaceTooFew verifies reconstruction fails when too few shards
// are present.
func TestReconstructInPlaceTooFew(t *testing.T) {
	data := randPayload(200, 5)
	k, m := 5, 3
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Present only k-1 shards.
	present := make([]bool, len(shards))
	for i := 0; i < k-1; i++ {
		present[i] = true
	}
	if err := ReconstructInPlace(shards, present, k); err != ErrTooFewShards {
		t.Fatalf("got %v, want ErrTooFewShards", err)
	}
}

// TestComputeParity verifies ComputeParity recomputes parity from data shards.
func TestComputeParity(t *testing.T) {
	data := randPayload(333, 9)
	k, m := 6, 3
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Corrupt the parity shards, then recompute them.
	for i := k; i < k+m; i++ {
		shards[i] = append([]byte(nil), shards[i]...)
		for j := range shards[i] {
			shards[i][j] ^= 0xFF
		}
	}
	if err := ComputeParity(shards, k, m); err != nil {
		t.Fatalf("ComputeParity: %v", err)
	}
	// Re-encode and compare: parity must match the freshly encoded parity.
	again, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	for i := k; i < k+m; i++ {
		if !bytes.Equal(shards[i], again[i]) {
			t.Fatalf("parity shard %d not recomputed correctly", i)
		}
	}
}

// TestBuildCodeMatrix checks the code matrix has identity rows for data shards.
func TestBuildCodeMatrix(t *testing.T) {
	k, m := 4, 2
	C, err := BuildCodeMatrix(k, m)
	if err != nil {
		t.Fatalf("BuildCodeMatrix: %v", err)
	}
	if len(C) != k+m || len(C[0]) != k {
		t.Fatalf("code matrix dimensions = %dx%d, want %dx%d", len(C), len(C[0]), k+m, k)
	}
	for i := 0; i < k; i++ {
		for j := 0; j < k; j++ {
			want := byte(0)
			if i == j {
				want = 1
			}
			if C[i][j] != want {
				t.Fatalf("C[%d][%d]=%d, want %d", i, j, C[i][j], want)
			}
		}
	}
	// Any k rows must be invertible.
	for start := 0; start+k <= k+m; start++ {
		sub := subMatrix(C, start, 0, k, k)
		if _, err := Invert(sub); err != nil {
			t.Fatalf("code matrix rows %d..%d not invertible: %v", start, start+k, err)
		}
	}
}
