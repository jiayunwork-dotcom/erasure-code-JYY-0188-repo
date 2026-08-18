# erasure-code

`erasure-code` is a small, dependency-free Go library and command-line tool that
implements Reed-Solomon erasure coding over the binary Galois field GF(2^8). It
splits a byte payload into a configurable number of **data shards** and computes
additional **parity shards** so that the original data can be recovered from any
subset of shards as long as at least the data-shard count survives. This makes it
useful for storage and transport scenarios where some pieces may be lost or
corrupted.

The field arithmetic uses the primitive polynomial `0x11D` and precomputed
exponent/logarithm tables plus a multiplication table for speed. Encoding is
performed with a Vandermonde matrix over GF(2^8); decoding (reconstruction) uses
Gauss-Jordan inversion of the submatrix formed by the available shards. All
operations are deterministic for a given input and shard configuration.

## Packages

- `internal/galois` — GF(2^8) arithmetic: `Add`, `Mul`, `Div`, `Inverse`,
  `Pow`, and a precomputed `MulTable`.
- `internal/codec` — Reed-Solomon encoding with a Vandermonde matrix.
- `internal/reedsolomon` — facade offering `Split`, `Encode`, `Reconstruct`,
  and `Verify`.

## Build and test

```sh
export GOTOOLCHAIN=local CGO_ENABLED=0
go build ./...
go test ./...
go vet ./...
```

## Command-line usage

```sh
# Encode a file into data + parity shards written to a directory.
erasure-code encode --data 4 --parity 2 input.bin outdir

# Recover the original from whichever shard files are present in the directory.
erasure-code reconstruct outdir
```

The `encode` subcommand writes one file per shard (`shard.000`, `shard.001`, …)
along with a `meta.json` describing the original size and shard counts. The
`reconstruct` subcommand reads the available shards, reconstructs any that are
missing, and reports which shards were used for recovery.

## Constraints honored

- Standard library only; no third-party dependencies.
- Total shard count is bounded by the field size (≤ 255).
- Bad input (non-positive shard counts, too few available shards, mismatched
  shard sizes) is reported through returned errors rather than panics.
