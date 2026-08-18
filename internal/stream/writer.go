package stream

import (
	"io"
)

// ShardWriter writes encoded shard data for a single shard index across all
// stripes. Each call to WriteShard appends one stripe's worth of shard data.
type ShardWriter struct {
	w         io.Writer
	shardSize int
	written   int
}

// NewShardWriter creates a ShardWriter that writes the header followed by shard
// chunks to w.
func NewShardWriter(w io.Writer, h Header) (*ShardWriter, error) {
	hdr := MarshalHeader(h)
	if _, err := w.Write(hdr); err != nil {
		return nil, err
	}
	stripeSize := int(h.StripeSize)
	shardSize := stripeSize / int(h.DataShards)
	return &ShardWriter{
		w:         w,
		shardSize: shardSize,
	}, nil
}

// WriteShard writes one stripe's shard data. The shard must be exactly shardSize
// bytes.
func (sw *ShardWriter) WriteShard(shard []byte) error {
	if len(shard) != sw.shardSize {
		return ErrShortWrite
	}
	n, err := sw.w.Write(shard)
	if err != nil {
		return err
	}
	if n != sw.shardSize {
		return ErrShortWrite
	}
	sw.written++
	return nil
}

// Written returns the number of shard chunks written so far.
func (sw *ShardWriter) Written() int { return sw.written }

// ShardSize returns the expected size of each shard chunk.
func (sw *ShardWriter) ShardSize() int { return sw.shardSize }

// MultiWriter manages multiple ShardWriters (one per shard index) and provides
// a convenience method to write an entire stripe's worth of shards at once.
type MultiWriter struct {
	writers []*ShardWriter
}

// NewMultiWriter creates a MultiWriter from a slice of io.Writers (one per shard
// index). It writes the header to each underlying writer.
func NewMultiWriter(ws []io.Writer, h Header) (*MultiWriter, error) {
	sws := make([]*ShardWriter, len(ws))
	for i, w := range ws {
		sw, err := NewShardWriter(w, h)
		if err != nil {
			return nil, err
		}
		sws[i] = sw
	}
	return &MultiWriter{writers: sws}, nil
}

// WriteStripe writes one shard from each shard index. shards must have the same
// length as the number of writers.
func (mw *MultiWriter) WriteStripe(shards [][]byte) error {
	if len(shards) != len(mw.writers) {
		return ErrShortWrite
	}
	for i, sw := range mw.writers {
		if err := sw.WriteShard(shards[i]); err != nil {
			return err
		}
	}
	return nil
}

// Flush is a no-op for basic io.Writers but allows callers to signal completion.
func (mw *MultiWriter) Flush() error {
	for _, sw := range mw.writers {
		if f, ok := sw.w.(interface{ Flush() error }); ok {
			if err := f.Flush(); err != nil {
				return err
			}
		}
	}
	return nil
}

// WrittenPerShard returns the number of stripes each shard writer has written.
func (mw *MultiWriter) WrittenPerShard() []int {
	counts := make([]int, len(mw.writers))
	for i, sw := range mw.writers {
		counts[i] = sw.Written()
	}
	return counts
}
