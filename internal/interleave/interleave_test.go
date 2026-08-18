package interleave

import (
	"bytes"
	"testing"
)

func TestEncodeReconstructRoundTrip(t *testing.T) {
	cfg := Config{DataShards: 3, ParityShards: 2, Depth: 4}
	data := make([]byte, cfg.BlockSize())
	for i := range data {
		data[i] = byte(i + 1)
	}
	encoded, err := Encode(data, cfg)
	if err != nil {
		t.Fatal(err)
	}
	total := cfg.TotalShards()
	if len(encoded) != total {
		t.Fatalf("expected %d rows, got %d", total, len(encoded))
	}
	// Verify data rows match input.
	reconstructed := make([]byte, 0, cfg.BlockSize())
	for r := 0; r < cfg.DataShards; r++ {
		reconstructed = append(reconstructed, encoded[r]...)
	}
	if !bytes.Equal(reconstructed, data) {
		t.Fatal("data rows do not match input")
	}
	// Lose 2 rows and reconstruct.
	encoded[0] = nil
	encoded[total-1] = nil
	present := make([]bool, total)
	for i := range encoded {
		present[i] = encoded[i] != nil
	}
	if err := Reconstruct(encoded, present, cfg); err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	// Verify data recovery.
	recovered := make([]byte, 0, cfg.BlockSize())
	for r := 0; r < cfg.DataShards; r++ {
		recovered = append(recovered, encoded[r]...)
	}
	if !bytes.Equal(recovered, data) {
		t.Fatal("recovered data does not match original")
	}
}

func TestBlockEncodeAndVerify(t *testing.T) {
	cfg := Config{DataShards: 4, ParityShards: 2, Depth: 8}
	blk := NewBlock(cfg, cfg.Depth)
	data := make([]byte, cfg.BlockSize())
	for i := range data {
		data[i] = byte(i * 3)
	}
	if err := blk.SetData(data); err != nil {
		t.Fatal(err)
	}
	if err := blk.Encode(); err != nil {
		t.Fatal(err)
	}
	ok, err := blk.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("block verify failed after encoding")
	}
}

func TestBlockReconstructAfterLoss(t *testing.T) {
	cfg := Config{DataShards: 3, ParityShards: 2, Depth: 6}
	blk := NewBlock(cfg, cfg.Depth)
	data := make([]byte, cfg.BlockSize())
	for i := range data {
		data[i] = byte(i + 10)
	}
	if err := blk.SetData(data); err != nil {
		t.Fatal(err)
	}
	if err := blk.Encode(); err != nil {
		t.Fatal(err)
	}
	original := blk.Clone()
	// Lose 2 rows.
	blk.MarkLost([]int{1, 4})
	if blk.CanRecover() == false {
		t.Fatal("should be recoverable")
	}
	if err := blk.Reconstruct(); err != nil {
		t.Fatal(err)
	}
	for i := range original.Rows {
		if !bytes.Equal(blk.Rows[i], original.Rows[i]) {
			t.Fatalf("row %d mismatch after reconstruction", i)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	valid := Config{DataShards: 4, ParityShards: 2, Depth: 8}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	invalid := Config{DataShards: 0, ParityShards: 2, Depth: 8}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected error for zero data shards")
	}
}
