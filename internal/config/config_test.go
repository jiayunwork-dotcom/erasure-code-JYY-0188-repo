package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileValidate(t *testing.T) {
	tests := []struct {
		name    string
		p       Profile
		wantErr bool
	}{
		{"valid", Profile{Name: "default", DataShards: 4, ParityShards: 2, StripeSize: 16}, false},
		{"no name", Profile{DataShards: 4, ParityShards: 2, StripeSize: 16}, true},
		{"zero data", Profile{Name: "x", DataShards: 0, ParityShards: 2, StripeSize: 16}, true},
		{"not divisible", Profile{Name: "x", DataShards: 4, ParityShards: 2, StripeSize: 15}, true},
		{"cauchy over 127", Profile{Name: "x", DataShards: 100, ParityShards: 30, StripeSize: 100, UseCauchy: true}, true},
		{"negative interleave", Profile{Name: "x", DataShards: 4, ParityShards: 2, StripeSize: 16, Interleave: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigFileSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "erasure.json")

	cf := &ConfigFile{
		Default: "main",
		Profiles: []Profile{
			{Name: "main", DataShards: 4, ParityShards: 2, StripeSize: 64},
			{Name: "high", DataShards: 8, ParityShards: 4, StripeSize: 64},
		},
	}
	if err := Save(path, cf); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Default != "main" {
		t.Fatalf("expected default 'main', got %q", loaded.Default)
	}
	if len(loaded.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(loaded.Profiles))
	}
	p := loaded.FindProfile("high")
	if p == nil {
		t.Fatal("profile 'high' not found")
	}
	if p.DataShards != 8 {
		t.Fatalf("expected 8 data shards, got %d", p.DataShards)
	}
}

func TestLoadNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err != ErrFileNotFound {
		t.Fatalf("expected ErrFileNotFound, got %v", err)
	}
}

func TestConfigFileValidate(t *testing.T) {
	cf := &ConfigFile{
		Profiles: []Profile{
			{Name: "a", DataShards: 4, ParityShards: 2, StripeSize: 16},
			{Name: "a", DataShards: 4, ParityShards: 2, StripeSize: 16},
		},
	}
	if err := cf.Validate(); err == nil {
		t.Fatal("expected error for duplicate profile names")
	}
}

func TestDefaultProfile(t *testing.T) {
	cf := &ConfigFile{
		Default: "second",
		Profiles: []Profile{
			{Name: "first", DataShards: 4, ParityShards: 2, StripeSize: 16},
			{Name: "second", DataShards: 8, ParityShards: 4, StripeSize: 32},
		},
	}
	p := cf.DefaultProfile()
	if p.Name != "second" {
		t.Fatalf("expected default profile 'second', got %q", p.Name)
	}
}

func TestProfileHelpers(t *testing.T) {
	p := &Profile{Name: "test", DataShards: 4, ParityShards: 2, StripeSize: 64}
	if p.ShardSize() != 16 {
		t.Fatalf("expected shard size 16, got %d", p.ShardSize())
	}
	if p.CanSurvive() != 2 {
		t.Fatalf("expected can survive 2, got %d", p.CanSurvive())
	}
	ratio := p.RedundancyRatio()
	if ratio < 0.49 || ratio > 0.51 {
		t.Fatalf("expected ratio ~0.5, got %f", ratio)
	}
}

func init() {
	// Silence unused import warning for os if it's needed elsewhere.
	_ = os.DevNull
}
