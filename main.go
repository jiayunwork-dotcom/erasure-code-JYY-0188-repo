// Command erasure-code is a command-line front end for the erasure-code library.
// It encodes a file into data and parity shards written to a directory, and
// reconstructs the original from whichever shard files survive in that
// directory.
//
// Usage:
//
//	erasure-code encode --data 4 --parity 2 <input> <outdir>
//	erasure-code reconstruct <outdir>
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"erasure-code/internal/reedsolomon"
)

// reorderArgs moves flag tokens (and their immediate values) to the front and
// leaves positional arguments at the end. The standard flag package stops
// parsing at the first non-flag argument, so placing flags first lets callers
// write either "cmd --flag val pos" or "cmd pos --flag val" and still have the
// flags parsed.
func reorderArgs(args []string) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		isFlag := strings.HasPrefix(a, "-") && a != "-" && a != "--"
		if isFlag {
			flags = append(flags, a)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, a)
	}
	return append(flags, positionals...)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "encode":
		runEncode(os.Args[2:])
	case "reconstruct":
		runReconstruct(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	case "info":
		runInfo(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  erasure-code encode --data N --parity M <input> <outdir>")
	fmt.Fprintln(os.Stderr, "  erasure-code reconstruct <outdir>")
	fmt.Fprintln(os.Stderr, "  erasure-code verify <outdir>")
	fmt.Fprintln(os.Stderr, "  erasure-code info <outdir>")
}

func runEncode(args []string) {
	fs := flag.NewFlagSet("encode", flag.ExitOnError)
	data := fs.Int("data", 0, "number of data shards")
	parity := fs.Int("parity", 0, "number of parity shards")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		os.Exit(2)
	}
	pos := fs.Args()
	if *data <= 0 || *parity <= 0 {
		fmt.Fprintln(os.Stderr, "encode: --data and --parity must be positive")
		os.Exit(2)
	}
	if len(pos) != 2 {
		fmt.Fprintln(os.Stderr, "encode: expected <input> <outdir>")
		os.Exit(2)
	}
	input, outdir := pos[0], pos[1]

	payload, err := os.ReadFile(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode: read %s: %v\n", input, err)
		os.Exit(1)
	}

	if err := reedsolomon.EncodeDir(payload, *data, *parity, outdir); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("encoded %d bytes into %d shards (%d data + %d parity) in %s\n",
		len(payload), *data+*parity, *data, *parity, outdir)
}

func runReconstruct(args []string) {
	fs := flag.NewFlagSet("reconstruct", flag.ExitOnError)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		os.Exit(2)
	}
	pos := fs.Args()
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "reconstruct: expected <outdir>")
		os.Exit(2)
	}
	outdir := pos[0]

	shards, present, m, err := reedsolomon.ReadShardsFromDir(outdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconstruct: %v\n", err)
		os.Exit(1)
	}

	if err := reedsolomon.Reconstruct(shards, present, m.DataShards); err != nil {
		fmt.Fprintf(os.Stderr, "reconstruct: %v\n", err)
		os.Exit(1)
	}

	recovered, err := reedsolomon.OriginalData(shards, m.DataShards, m.OriginalSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconstruct: %v\n", err)
		os.Exit(1)
	}
	outPath := filepath.Join(outdir, "recovered.bin")
	if err := os.WriteFile(outPath, recovered, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "reconstruct: write %s: %v\n", outPath, err)
		os.Exit(1)
	}

	used, err := reedsolomon.UsedShards(outdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconstruct: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("recovered %d bytes; used shards: %v\n", len(recovered), used)
	ok, err := reedsolomon.Verify(shards, m.DataShards)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconstruct: verify: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("verify: %v\n", ok)
}
