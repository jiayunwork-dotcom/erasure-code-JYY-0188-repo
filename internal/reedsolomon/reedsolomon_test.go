package reedsolomon

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func randPayload(n, seed int) []byte {
	rng := rand.New(rand.NewSource(int64(seed)))
	b := make([]byte, n)
	rng.Read(b)
	return b
}

// allPresent returns a present slice with every entry true.
func allPresent(n int) []bool {
	p := make([]bool, n)
	for i := range p {
		p[i] = true
	}
	return p
}

// TestSplit verifies Split produces the right number of shards and preserves the
// (padded) payload in the data shards.
func TestSplit(t *testing.T) {
	data := []byte("split me into equal shards please")
	k, m := 5, 3
	shards, err := Split(data, k, m)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(shards) != k+m {
		t.Fatalf("got %d shards, want %d", len(shards), k+m)
	}
	size := len(shards[0])
	for i := range shards {
		if len(shards[i]) != size {
			t.Fatalf("shard %d length %d != %d", i, len(shards[i]), size)
		}
	}
	// Data shards hold the padded payload.
	joined := bytes.Join(shards[:k], nil)
	if !bytes.Equal(joined[:len(data)], data) {
		t.Fatal("data shards do not preserve payload")
	}
	// Parity shards start zeroed.
	for i := k; i < k+m; i++ {
		for _, b := range shards[i] {
			if b != 0 {
				t.Fatalf("parity shard %d not zeroed", i)
			}
		}
	}
}

// TestSplitErrors verifies invalid shard counts are rejected.
func TestSplitErrors(t *testing.T) {
	if _, err := Split([]byte{1}, 0, 2); err != ErrInvalidShardCount {
		t.Fatalf("zero data shards: got %v", err)
	}
	if _, err := Split([]byte{1}, 4, 0); err != ErrInvalidShardCount {
		t.Fatalf("zero parity shards: got %v", err)
	}
	if _, err := Split([]byte{1}, 200, 100); err != ErrInvalidShardCount {
		t.Fatalf("too many total: got %v", err)
	}
}

// TestEncodeDeterministic checks the facade encodes deterministically.
func TestEncodeDeterministic(t *testing.T) {
	data := randPayload(512, 3)
	k, m := 7, 3
	a, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	b, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			t.Fatalf("shard %d differs between runs", i)
		}
	}
}

// TestReconstructMissing drops various combinations of data and parity shards and
// verifies reconstruction recovers the full set exactly.
func TestReconstructMissing(t *testing.T) {
	data := randPayload(1000, 21)
	k, m := 8, 4
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	orig := make([][]byte, len(shards))
	for i := range shards {
		orig[i] = append([]byte(nil), shards[i]...)
	}

	patterns := [][]int{
		{0},
		{k + 1},
		{0, k},
		{1, 2, k + 1},
		{0, 1, k, k + 1},
	}
	for _, missing := range patterns {
		present := allPresent(len(shards))
		for _, idx := range missing {
			present[idx] = false
		}
		if err := Reconstruct(shards, present, k); err != nil {
			t.Fatalf("missing %v: %v", missing, err)
		}
		for i := range shards {
			if !bytes.Equal(shards[i], orig[i]) {
				t.Fatalf("missing %v: shard %d not recovered", missing, i)
			}
		}
	}
}

// TestReconstructInsufficient verifies reconstruction fails when fewer than
// dataShards shards are present.
func TestReconstructInsufficient(t *testing.T) {
	data := randPayload(300, 33)
	k, m := 6, 3
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	present := allPresent(len(shards))
	// Keep only k-1 shards.
	for i := k - 1; i < len(shards); i++ {
		present[i] = false
	}
	if err := Reconstruct(shards, present, k); err != ErrTooFewShards {
		t.Fatalf("got %v, want ErrTooFewShards", err)
	}
}

// TestReconstructPresentMismatch verifies a present-length mismatch is rejected.
func TestReconstructPresentMismatch(t *testing.T) {
	shards, err := Encode(randPayload(100, 1), 4, 2)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	bad := make([]bool, len(shards)+1)
	if err := Reconstruct(shards, bad, 4); err != ErrPresentMismatch {
		t.Fatalf("got %v, want ErrPresentMismatch", err)
	}
}

// TestVerifyOK verifies Verify returns true for a correctly encoded set.
func TestVerifyOK(t *testing.T) {
	data := randPayload(640, 77)
	k, m := 6, 3
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	ok, err := Verify(shards, k)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("Verify returned false for valid shards")
	}
}

// TestVerifyFail verifies Verify detects a corrupted parity shard.
func TestVerifyFail(t *testing.T) {
	data := randPayload(640, 78)
	k, m := 6, 3
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// Corrupt a parity shard.
	shards[k][0] ^= 0xAB
	ok, err := Verify(shards, k)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Fatal("Verify returned true for corrupted shards")
	}
}

// TestOriginalData verifies the helper restores the original payload length.
func TestOriginalData(t *testing.T) {
	data := randPayload(250, 99)
	k, m := 5, 2
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := OriginalData(shards, k, len(data))
	if err != nil {
		t.Fatalf("OriginalData: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatal("OriginalData did not reproduce the payload")
	}
	if _, err := OriginalData(shards, k, len(data)+5); err == nil {
		t.Fatal("OriginalData should reject out-of-range size")
	}
}

// TestReconstructThenVerify encodes, drops shards, reconstructs, and verifies the
// result is consistent.
func TestReconstructThenVerify(t *testing.T) {
	data := randPayload(1234, 123)
	k, m := 10, 5
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	present := allPresent(len(shards))
	present[0] = false
	present[3] = false
	present[k+2] = false
	if err := Reconstruct(shards, present, k); err != nil {
		t.Fatalf("Reconstruct: %v", err)
	}
	ok, err := Verify(shards, k)
	if err != nil || !ok {
		t.Fatalf("post-reconstruction Verify ok=%v err=%v", ok, err)
	}
	out, err := OriginalData(shards, k, len(data))
	if err != nil {
		t.Fatalf("OriginalData: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatal("reconstructed data does not match original")
	}
}

// TestReconstructAllPresent verifies reconstruction with every shard available is
// a no-op that preserves the data.
func TestReconstructAllPresent(t *testing.T) {
	data := randPayload(400, 5)
	k, m := 7, 3
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	orig := make([][]byte, len(shards))
	for i := range shards {
		orig[i] = append([]byte(nil), shards[i]...)
	}
	if err := Reconstruct(shards, allPresent(len(shards)), k); err != nil {
		t.Fatalf("Reconstruct: %v", err)
	}
	for i := range shards {
		if !bytes.Equal(shards[i], orig[i]) {
			t.Fatalf("all-present reconstruction altered shard %d", i)
		}
	}
}

// TestEncodeDirReconstructDir writes shards to a temp directory, deletes a mix of
// shards, and reconstructs the original payload via the directory helpers.
func TestEncodeDirReconstructDir(t *testing.T) {
	data := randPayload(900, 555)
	k, m := 8, 4
	dir := t.TempDir()
	if err := EncodeDir(data, k, m, dir); err != nil {
		t.Fatalf("EncodeDir: %v", err)
	}
	// Confirm the metadata and shard files exist.
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
		t.Fatalf("meta.json missing: %v", err)
	}
	// Remove three shards (two data, one parity); still enough to recover.
	for _, idx := range []int{0, 3, k + 1} {
		if err := os.Remove(filepath.Join(dir, fmt.Sprintf("shard.%03d", idx))); err != nil {
			t.Fatalf("remove shard %d: %v", idx, err)
		}
	}
	used, err := UsedShards(dir)
	if err != nil {
		t.Fatalf("UsedShards: %v", err)
	}
	if len(used) != k+m-3 {
		t.Fatalf("UsedShards returned %d, want %d", len(used), k+m-3)
	}
	recovered, err := ReconstructDir(dir)
	if err != nil {
		t.Fatalf("ReconstructDir: %v", err)
	}
	if !bytes.Equal(recovered, data) {
		t.Fatal("ReconstructDir did not recover the original payload")
	}
}

// TestVerifyDir checks VerifyDir reports consistency for freshly encoded shards
// and inconsistency after corrupting a parity shard on disk.
func TestVerifyDir(t *testing.T) {
	data := randPayload(450, 222)
	k, m := 6, 3
	dir := t.TempDir()
	if err := EncodeDir(data, k, m, dir); err != nil {
		t.Fatalf("EncodeDir: %v", err)
	}
	ok, err := VerifyDir(dir)
	if err != nil {
		t.Fatalf("VerifyDir: %v", err)
	}
	if !ok {
		t.Fatal("VerifyDir reported inconsistent fresh shards")
	}
	// Corrupt a parity shard on disk.
	pname := filepath.Join(dir, fmt.Sprintf("shard.%03d", k))
	b, err := os.ReadFile(pname)
	if err != nil {
		t.Fatalf("read parity shard: %v", err)
	}
	b[0] ^= 0x55
	if err := os.WriteFile(pname, b, 0o644); err != nil {
		t.Fatalf("write corrupted shard: %v", err)
	}
	ok, err = VerifyDir(dir)
	if err != nil {
		t.Fatalf("VerifyDir after corruption: %v", err)
	}
	if ok {
		t.Fatal("VerifyDir should report inconsistency after corruption")
	}
}

// TestWriteReadShardsRoundtrip verifies WriteShardsToDir/ReadShardsFromDir
// preserve shards and the presence map.
func TestWriteReadShardsRoundtrip(t *testing.T) {
	data := randPayload(300, 321)
	k, m := 5, 2
	shards, err := Encode(data, k, m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	dir := t.TempDir()
	meta := ShardMeta{OriginalSize: len(data), DataShards: k, ParityShards: m}
	if err := WriteShardsToDir(shards, meta, dir); err != nil {
		t.Fatalf("WriteShardsToDir: %v", err)
	}
	got, present, gotMeta, err := ReadShardsFromDir(dir)
	if err != nil {
		t.Fatalf("ReadShardsFromDir: %v", err)
	}
	if gotMeta != meta {
		t.Fatalf("meta mismatch: %+v vs %+v", gotMeta, meta)
	}
	if len(got) != k+m {
		t.Fatalf("read %d shards, want %d", len(got), k+m)
	}
	for i := range got {
		if !present[i] {
			t.Fatalf("shard %d reported absent", i)
		}
		if !bytes.Equal(got[i], shards[i]) {
			t.Fatalf("shard %d not preserved", i)
		}
	}
}

// TestReadShardsFromDirMissing verifies the presence map marks deleted shards.
func TestReadShardsFromDirMissing(t *testing.T) {
	data := randPayload(200, 44)
	k, m := 4, 2
	dir := t.TempDir()
	if err := EncodeDir(data, k, m, dir); err != nil {
		t.Fatalf("EncodeDir: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "shard.000")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, fmt.Sprintf("shard.%03d", k))); err != nil {
		t.Fatalf("remove: %v", err)
	}
	_, present, _, err := ReadShardsFromDir(dir)
	if err != nil {
		t.Fatalf("ReadShardsFromDir: %v", err)
	}
	if present[0] || present[k] {
		t.Fatal("deleted shards should be marked absent")
	}
	if !present[1] || !present[k+1] {
		t.Fatal("present shards should be marked present")
	}
}
