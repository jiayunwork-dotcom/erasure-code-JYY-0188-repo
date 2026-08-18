package stream

import (
	"encoding/binary"
	"errors"
	"io"
)

// Header is written at the start of each shard stream to describe the encoding
// parameters and the original data length.
type Header struct {
	DataShards   uint8
	ParityShards uint8
	StripeSize   uint32
	OriginalSize uint64
}

// headerSize is the fixed byte count of a serialized Header.
const headerSize = 1 + 1 + 4 + 8

// ErrBadHeader is returned when a shard stream header is malformed.
var ErrBadHeader = errors.New("stream: malformed header")

// MarshalHeader serializes a Header into a fixed-size byte slice.
func MarshalHeader(h Header) []byte {
	buf := make([]byte, headerSize)
	buf[0] = h.DataShards
	buf[1] = h.ParityShards
	binary.BigEndian.PutUint32(buf[2:6], h.StripeSize)
	binary.BigEndian.PutUint64(buf[6:14], h.OriginalSize)
	return buf
}

// UnmarshalHeader reads a Header from the first headerSize bytes.
func UnmarshalHeader(buf []byte) (Header, error) {
	if len(buf) < headerSize {
		return Header{}, ErrBadHeader
	}
	h := Header{
		DataShards:   buf[0],
		ParityShards: buf[1],
		StripeSize:   binary.BigEndian.Uint32(buf[2:6]),
		OriginalSize: binary.BigEndian.Uint64(buf[6:14]),
	}
	if h.DataShards == 0 || h.ParityShards == 0 || h.StripeSize == 0 {
		return Header{}, ErrBadHeader
	}
	return h, nil
}

// ShardReader reads encoded shard data from an io.Reader one stripe at a time.
// It expects a Header at the beginning of the stream followed by shard chunks of
// fixed size.
type ShardReader struct {
	r         io.Reader
	header    Header
	shardSize int
	stripe    int // current stripe index
	totalStr  int // total stripes expected
	done      bool
}

// NewShardReader creates a ShardReader by reading and parsing the header from r.
func NewShardReader(r io.Reader) (*ShardReader, error) {
	buf := make([]byte, headerSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	h, err := UnmarshalHeader(buf)
	if err != nil {
		return nil, err
	}
	stripeSize := int(h.StripeSize)
	shardSize := stripeSize / int(h.DataShards)
	totalStripes := int((h.OriginalSize + uint64(stripeSize) - 1) / uint64(stripeSize))
	return &ShardReader{
		r:         r,
		header:    h,
		shardSize: shardSize,
		totalStr:  totalStripes,
	}, nil
}

// Header returns the parsed header.
func (sr *ShardReader) Header() Header { return sr.header }

// ReadShard reads one shard chunk for the current stripe.
func (sr *ShardReader) ReadShard() ([]byte, error) {
	if sr.done {
		return nil, io.EOF
	}
	buf := make([]byte, sr.shardSize)
	n, err := io.ReadFull(sr.r, buf)
	if n == 0 && err != nil {
		sr.done = true
		return nil, io.EOF
	}
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	// Zero-pad short final read.
	if n < sr.shardSize {
		for i := n; i < sr.shardSize; i++ {
			buf[i] = 0
		}
	}
	return buf, nil
}

// Remaining returns the number of stripes not yet read.
func (sr *ShardReader) Remaining() int {
	left := sr.totalStr - sr.stripe
	if left < 0 {
		return 0
	}
	return left
}

// Advance moves the stripe counter forward (call after processing a full stripe).
func (sr *ShardReader) Advance() {
	sr.stripe++
	if sr.stripe >= sr.totalStr {
		sr.done = true
	}
}
