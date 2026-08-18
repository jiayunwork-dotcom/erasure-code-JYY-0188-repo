package cauchy

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestCauchyMatrixInvertible(t *testing.T) {
	data, parity := 4, 2
	cm, err := CauchyMatrix(data, parity)
	if err != nil {
		t.Fatal(err)
	}
	if cm.Rows() != parity || cm.Cols() != data {
		t.Fatalf("unexpected matrix dimensions: %dx%d", cm.Rows(), cm.Cols())
	}
	// Every element should be non-zero (Cauchy property).
	for r := 0; r < parity; r++ {
		for c := 0; c < data; c++ {
			if cm.Get(r, c) == 0 {
				t.Fatalf("zero element at (%d,%d)", r, c)
			}
		}
	}
}

func TestEncodeVerify(t *testing.T) {
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	shards, err := Encode(data, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 6 {
		t.Fatalf("expected 6 shards, got %d", len(shards))
	}
	ok, err := Verify(shards, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("verify failed on fresh encoding")
	}
}

func TestCauchyReconstructPartial(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dataShards := 6
	parityShards := 3
	payload := make([]byte, 300)
	rng.Read(payload)

	shards, err := Encode(payload, dataShards, parityShards)
	if err != nil {
		t.Fatal(err)
	}
	total := dataShards + parityShards

	lossPatterns := [][]int{
		{0},
		{dataShards},
		{2, 5},
		{0, dataShards + 1},
		{1, 4, dataShards + 2},
	}
	for _, lost := range lossPatterns {
		// Deep copy shards.
		test := make([][]byte, total)
		present := make([]bool, total)
		for i := range shards {
			test[i] = make([]byte, len(shards[i]))
			copy(test[i], shards[i])
			present[i] = true
		}
		for _, idx := range lost {
			test[idx] = nil
			present[idx] = false
		}
		if err := Reconstruct(test, present, dataShards); err != nil {
			t.Fatalf("reconstruct failed for lost=%v: %v", lost, err)
		}
		// Verify all shards match original.
		for i := 0; i < total; i++ {
			if !bytes.Equal(test[i], shards[i]) {
				t.Fatalf("shard %d mismatch after losing %v", i, lost)
			}
		}
	}
}

func TestCauchyInvalidParams(t *testing.T) {
	_, err := Encode(nil, 4, 2)
	if err != ErrInvalidParams {
		t.Fatalf("expected ErrInvalidParams for nil data, got %v", err)
	}
	_, err = Encode([]byte("x"), 0, 2)
	if err != ErrInvalidParams {
		t.Fatalf("expected ErrInvalidParams for zero data shards, got %v", err)
	}
}

func TestFullMatrixIdentityTop(t *testing.T) {
	full, err := FullMatrix(4, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Top 4 rows should be identity.
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			want := byte(0)
			if i == j {
				want = 1
			}
			if full.Get(i, j) != want {
				t.Fatalf("identity row %d col %d: got %d, want %d", i, j, full.Get(i, j), want)
			}
		}
	}
}
