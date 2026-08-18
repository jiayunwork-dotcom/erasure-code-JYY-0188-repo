package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"erasure-code/internal/reedsolomon"
)

// runVerify checks shard integrity for an encoded directory.
func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		os.Exit(2)
	}
	pos := fs.Args()
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "verify: expected <outdir>")
		os.Exit(2)
	}
	outdir := pos[0]

	ok, err := reedsolomon.VerifyDir(outdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		os.Exit(1)
	}
	if ok {
		fmt.Println("verify: all shards are consistent")
	} else {
		fmt.Println("verify: INCONSISTENT — parity does not match data")
		os.Exit(1)
	}
}

// runInfo prints metadata and shard presence for an encoded directory.
func runInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		os.Exit(2)
	}
	pos := fs.Args()
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "info: expected <outdir>")
		os.Exit(2)
	}
	outdir := pos[0]

	metaPath := filepath.Join(outdir, "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "info: %v\n", err)
		os.Exit(1)
	}

	var m reedsolomon.ShardMeta
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "info: parse meta: %v\n", err)
		os.Exit(1)
	}

	total := m.DataShards + m.ParityShards
	fmt.Printf("directory:     %s\n", outdir)
	fmt.Printf("original_size: %d bytes\n", m.OriginalSize)
	fmt.Printf("data_shards:   %d\n", m.DataShards)
	fmt.Printf("parity_shards: %d\n", m.ParityShards)
	fmt.Printf("total_shards:  %d\n", total)
	fmt.Printf("fault_tolerance: %d shard(s)\n", m.ParityShards)

	// Count present shards.
	present := 0
	for i := 0; i < total; i++ {
		name := filepath.Join(outdir, fmt.Sprintf("shard.%03d", i))
		if _, serr := os.Stat(name); serr == nil {
			present++
		}
	}
	fmt.Printf("present:       %d / %d\n", present, total)
	missing := total - present
	if missing > 0 {
		fmt.Printf("missing:       %d\n", missing)
		if missing > m.ParityShards {
			fmt.Println("status:        UNRECOVERABLE")
		} else {
			fmt.Println("status:        RECOVERABLE")
		}
	} else {
		fmt.Println("status:        HEALTHY")
	}
}
