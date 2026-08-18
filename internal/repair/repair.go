// Package repair provides incremental repair scheduling for distributed erasure-
// coded storage. When multiple shards of a stripe are lost, the scheduler
// determines the minimum set of surviving shards to read and the order in which
// to rebuild missing shards. This minimizes network I/O and disk reads during
// recovery.
package repair

import (
	"errors"
	"sort"
)

// ErrInvalidLayout is returned when the stripe layout is inconsistent.
var ErrInvalidLayout = errors.New("repair: invalid layout")

// ErrUnrecoverable is returned when too few shards survive to reconstruct.
var ErrUnrecoverable = errors.New("repair: unrecoverable stripe")

// ErrNoRepairNeeded is returned when all shards are present.
var ErrNoRepairNeeded = errors.New("repair: no repair needed")

// ShardStatus describes whether a shard is present, missing, or degraded.
type ShardStatus int

const (
	// Present means the shard is available and verified.
	Present ShardStatus = iota
	// Missing means the shard is completely unavailable.
	Missing
	// Degraded means the shard is readable but has integrity issues.
	Degraded
)

// String implements fmt.Stringer for ShardStatus.
func (s ShardStatus) String() string {
	switch s {
	case Present:
		return "present"
	case Missing:
		return "missing"
	case Degraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// Stripe describes the layout of one erasure-coded stripe.
type Stripe struct {
	DataShards   int           // number of data shards
	ParityShards int           // number of parity shards
	Status       []ShardStatus // per-shard status; length == DataShards+ParityShards
}

// Total returns the total shard count.
func (s *Stripe) Total() int { return s.DataShards + s.ParityShards }

// Validate checks that the stripe layout is consistent.
func (s *Stripe) Validate() error {
	if s.DataShards <= 0 || s.ParityShards <= 0 {
		return ErrInvalidLayout
	}
	if len(s.Status) != s.Total() {
		return ErrInvalidLayout
	}
	return nil
}

// AvailableCount returns the number of present + degraded shards (usable for
// reconstruction).
func (s *Stripe) AvailableCount() int {
	count := 0
	for _, st := range s.Status {
		if st == Present || st == Degraded {
			count++
		}
	}
	return count
}

// MissingIndices returns sorted indices of missing shards.
func (s *Stripe) MissingIndices() []int {
	var idxs []int
	for i, st := range s.Status {
		if st == Missing {
			idxs = append(idxs, i)
		}
	}
	sort.Ints(idxs)
	return idxs
}

// DegradedIndices returns sorted indices of degraded shards.
func (s *Stripe) DegradedIndices() []int {
	var idxs []int
	for i, st := range s.Status {
		if st == Degraded {
			idxs = append(idxs, i)
		}
	}
	sort.Ints(idxs)
	return idxs
}

// IsHealthy returns true if all shards are present and none are degraded.
func (s *Stripe) IsHealthy() bool {
	for _, st := range s.Status {
		if st != Present {
			return false
		}
	}
	return true
}

// RepairPlan describes which shards to read and which to rebuild.
type RepairPlan struct {
	// ReadFrom lists shard indices from which data will be read for
	// reconstruction.
	ReadFrom []int
	// Rebuild lists shard indices that will be reconstructed.
	Rebuild []int
	// Priority indicates the urgency: higher means more redundancy lost.
	Priority int
}

// Plan computes a repair plan for the given stripe. It selects the minimum
// set of surviving shards needed to reconstruct all missing ones. Degraded
// shards are treated as usable for reconstruction but scheduled for rebuild
// as well. Returns ErrNoRepairNeeded if the stripe is healthy, or
// ErrUnrecoverable if too few shards survive.
func Plan(stripe *Stripe) (*RepairPlan, error) {
	if err := stripe.Validate(); err != nil {
		return nil, err
	}
	if stripe.IsHealthy() {
		return nil, ErrNoRepairNeeded
	}

	available := stripe.AvailableCount()
	if available < stripe.DataShards {
		return nil, ErrUnrecoverable
	}

	missing := stripe.MissingIndices()
	degraded := stripe.DegradedIndices()

	// Select exactly DataShards shards to read from. Prefer present over
	// degraded; among equal status prefer lower index for determinism.
	readFrom := make([]int, 0, stripe.DataShards)
	for i, st := range stripe.Status {
		if st == Present && len(readFrom) < stripe.DataShards {
			readFrom = append(readFrom, i)
		}
	}
	// If not enough present shards, add degraded.
	for i, st := range stripe.Status {
		if st == Degraded && len(readFrom) < stripe.DataShards {
			readFrom = append(readFrom, i)
		}
	}
	sort.Ints(readFrom)

	// Rebuild = missing + degraded.
	rebuild := make([]int, 0, len(missing)+len(degraded))
	rebuild = append(rebuild, missing...)
	rebuild = append(rebuild, degraded...)
	sort.Ints(rebuild)

	// Priority = number of lost/degraded shards. Maximum is ParityShards (at
	// which point we have zero redundancy left).
	priority := len(missing) + len(degraded)
	if priority > stripe.ParityShards {
		priority = stripe.ParityShards
	}

	return &RepairPlan{
		ReadFrom: readFrom,
		Rebuild:  rebuild,
		Priority: priority,
	}, nil
}
