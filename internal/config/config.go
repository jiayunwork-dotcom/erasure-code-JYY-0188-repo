// Package config provides configuration parsing and validation for the erasure-
// code command-line tool and library consumers. It handles JSON-based profile
// files that specify encoding parameters, stripe sizes, and repair policies.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ErrInvalidProfile is returned when a profile has invalid or inconsistent fields.
var ErrInvalidProfile = errors.New("config: invalid profile")

// ErrFileNotFound is returned when the profile file does not exist.
var ErrFileNotFound = errors.New("config: file not found")

// Profile describes a named encoding configuration.
type Profile struct {
	Name         string `json:"name"`
	DataShards   int    `json:"data_shards"`
	ParityShards int    `json:"parity_shards"`
	StripeSize   int    `json:"stripe_size"`     // bytes; must be divisible by DataShards
	Interleave   int    `json:"interleave"`      // 0 = disabled, >0 = depth
	UseCauchy    bool   `json:"use_cauchy"`      // use Cauchy matrix instead of Vandermonde
	MaxRepairBW  int    `json:"max_repair_bw"`   // max concurrent repair reads (0 = unlimited)
}

// Validate checks that the profile is consistent and within bounds.
func (p *Profile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidProfile)
	}
	if p.DataShards <= 0 || p.ParityShards <= 0 {
		return fmt.Errorf("%w: data_shards and parity_shards must be positive", ErrInvalidProfile)
	}
	total := p.DataShards + p.ParityShards
	if total > 255 {
		return fmt.Errorf("%w: total shards exceeds 255", ErrInvalidProfile)
	}
	if p.UseCauchy && total > 127 {
		return fmt.Errorf("%w: cauchy mode limits total to 127", ErrInvalidProfile)
	}
	if p.StripeSize <= 0 {
		return fmt.Errorf("%w: stripe_size must be positive", ErrInvalidProfile)
	}
	if p.StripeSize%p.DataShards != 0 {
		return fmt.Errorf("%w: stripe_size must be divisible by data_shards", ErrInvalidProfile)
	}
	if p.Interleave < 0 {
		return fmt.Errorf("%w: interleave must be non-negative", ErrInvalidProfile)
	}
	if p.MaxRepairBW < 0 {
		return fmt.Errorf("%w: max_repair_bw must be non-negative", ErrInvalidProfile)
	}
	return nil
}

// ShardSize returns the per-shard size for the profile.
func (p *Profile) ShardSize() int {
	return p.StripeSize / p.DataShards
}

// RedundancyRatio returns the ratio of parity to data shards as a float.
func (p *Profile) RedundancyRatio() float64 {
	return float64(p.ParityShards) / float64(p.DataShards)
}

// CanSurvive returns the maximum number of simultaneous shard losses this
// configuration can tolerate.
func (p *Profile) CanSurvive() int {
	return p.ParityShards
}

// ConfigFile is the on-disk representation of a collection of profiles.
type ConfigFile struct {
	Default  string    `json:"default"`
	Profiles []Profile `json:"profiles"`
}

// FindProfile searches for a profile by name. Returns nil if not found.
func (cf *ConfigFile) FindProfile(name string) *Profile {
	for i := range cf.Profiles {
		if cf.Profiles[i].Name == name {
			return &cf.Profiles[i]
		}
	}
	return nil
}

// DefaultProfile returns the profile marked as default. If no default is set or
// the named profile doesn't exist, returns the first profile or nil.
func (cf *ConfigFile) DefaultProfile() *Profile {
	if cf.Default != "" {
		if p := cf.FindProfile(cf.Default); p != nil {
			return p
		}
	}
	if len(cf.Profiles) > 0 {
		return &cf.Profiles[0]
	}
	return nil
}

// Validate checks all profiles in the file.
func (cf *ConfigFile) Validate() error {
	if len(cf.Profiles) == 0 {
		return fmt.Errorf("%w: no profiles defined", ErrInvalidProfile)
	}
	seen := make(map[string]bool)
	for i := range cf.Profiles {
		if err := cf.Profiles[i].Validate(); err != nil {
			return err
		}
		if seen[cf.Profiles[i].Name] {
			return fmt.Errorf("%w: duplicate profile name %q", ErrInvalidProfile, cf.Profiles[i].Name)
		}
		seen[cf.Profiles[i].Name] = true
	}
	return nil
}

// Load reads and parses a ConfigFile from the given path.
func Load(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cf ConfigFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &cf, nil
}

// Save writes a ConfigFile to the given path with indentation.
func Save(path string, cf *ConfigFile) error {
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
