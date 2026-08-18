package interleave

import (
	"errors"
)

// Block represents a complete interleaved encoding unit: a (DataShards+ParityShards)
// x Depth matrix of bytes.
type Block struct {
	Rows  [][]byte
	Cfg   Config
	Depth int
}

// ErrBlockCorrupt is returned when a block has structural issues.
var ErrBlockCorrupt = errors.New("interleave: block is corrupt")

// NewBlock creates an empty Block with the given config and depth.
func NewBlock(cfg Config, depth int) *Block {
	total := cfg.TotalShards()
	rows := make([][]byte, total)
	for i := range rows {
		rows[i] = make([]byte, depth)
	}
	return &Block{Rows: rows, Cfg: cfg, Depth: depth}
}

// SetData fills the data rows of the block from flat data bytes.
func (b *Block) SetData(data []byte) error {
	needed := b.Cfg.DataShards * b.Depth
	if len(data) < needed {
		return ErrDataTooShort
	}
	for r := 0; r < b.Cfg.DataShards; r++ {
		copy(b.Rows[r], data[r*b.Depth:(r+1)*b.Depth])
	}
	return nil
}

// Data returns the flat data bytes from the data rows.
func (b *Block) Data() []byte {
	out := make([]byte, 0, b.Cfg.DataShards*b.Depth)
	for r := 0; r < b.Cfg.DataShards; r++ {
		out = append(out, b.Rows[r]...)
	}
	return out
}

// Encode computes parity rows from the data rows. The block must have its data
// rows populated before calling Encode.
func (b *Block) Encode() error {
	data := b.Data()
	encoded, err := Encode(data, b.Cfg)
	if err != nil {
		return err
	}
	for i := range encoded {
		b.Rows[i] = encoded[i]
	}
	return nil
}

// Reconstruct recovers missing rows. Mark missing rows with nil before calling.
func (b *Block) Reconstruct() error {
	present := make([]bool, len(b.Rows))
	for i, row := range b.Rows {
		present[i] = row != nil && len(row) == b.Depth
	}
	return Reconstruct(b.Rows, present, b.Cfg)
}

// Verify checks that parity rows are consistent with data rows.
func (b *Block) Verify() (bool, error) {
	// Re-encode and compare parity rows.
	data := b.Data()
	encoded, err := Encode(data, b.Cfg)
	if err != nil {
		return false, err
	}
	for p := 0; p < b.Cfg.ParityShards; p++ {
		row := b.Cfg.DataShards + p
		for col := 0; col < b.Depth; col++ {
			if b.Rows[row][col] != encoded[row][col] {
				return false, nil
			}
		}
	}
	return true, nil
}

// Missing returns the indices of nil or empty rows in the block.
func (b *Block) Missing() []int {
	var missing []int
	for i, row := range b.Rows {
		if row == nil || len(row) != b.Depth {
			missing = append(missing, i)
		}
	}
	return missing
}

// Clone returns a deep copy of the block.
func (b *Block) Clone() *Block {
	nb := NewBlock(b.Cfg, b.Depth)
	for i := range b.Rows {
		copy(nb.Rows[i], b.Rows[i])
	}
	return nb
}

// MarkLost sets the specified row indices to nil, simulating shard loss.
func (b *Block) MarkLost(indices []int) {
	for _, idx := range indices {
		if idx >= 0 && idx < len(b.Rows) {
			b.Rows[idx] = nil
		}
	}
}

// AvailableCount returns the number of non-nil, correctly sized rows.
func (b *Block) AvailableCount() int {
	count := 0
	for _, row := range b.Rows {
		if row != nil && len(row) == b.Depth {
			count++
		}
	}
	return count
}

// CanRecover returns true if enough rows are present to recover the block.
func (b *Block) CanRecover() bool {
	return b.AvailableCount() >= b.Cfg.DataShards
}
