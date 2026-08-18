package codec

import (
	"errors"

	"erasure-code/internal/galois"
)

// ErrInvalidShardCount is returned when a shard count is non-positive or the
// total exceeds the field size.
var ErrInvalidShardCount = errors.New("codec: invalid shard count")

// ErrMatrixNotInvertible is returned when a matrix that must be invertible
// (e.g. a Vandermonde submatrix) is singular over GF(2^8).
var ErrMatrixNotInvertible = errors.New("codec: matrix is not invertible")

// maxTotalShards is the largest allowed total shard count. It is one less than
// the field size because the generator values used to build the Vandermonde
// matrix are 1..total, all of which must be distinct non-zero field elements.
const maxTotalShards = 255

// matrix is a row-major 2D byte slice used for GF(2^8) matrix arithmetic.
type matrix [][]byte

// newMatrix allocates a rows x cols zero matrix.
func newMatrix(rows, cols int) matrix {
	m := make(matrix, rows)
	for r := 0; r < rows; r++ {
		m[r] = make([]byte, cols)
	}
	return m
}

// vandermonde builds a rows x cols Vandermonde matrix V where
// V[r][c] = Pow(byte(r+1), c). The generator values r+1 are distinct and
// non-zero for r in [0, rows-1], which makes any square subset invertible.
func vandermonde(rows, cols int) matrix {
	vm := newMatrix(rows, cols)
	for r := 0; r < rows; r++ {
		base := byte(r + 1)
		for c := 0; c < cols; c++ {
			vm[r][c] = galois.Pow(base, c)
		}
	}
	return vm
}

// subMatrix returns the submatrix spanning rows [r0, r0+rs) and columns
// [c0, c0+cs).
func subMatrix(m matrix, r0, c0, rs, cs int) matrix {
	out := newMatrix(rs, cs)
	for r := 0; r < rs; r++ {
		copy(out[r], m[r0+r][c0:c0+cs])
	}
	return out
}

// multiply returns a*b for two matrices with compatible dimensions.
func multiply(a, b matrix) (matrix, error) {
	if len(a) == 0 || len(b) == 0 {
		return nil, errors.New("codec: cannot multiply empty matrix")
	}
	ar, ac := len(a), len(a[0])
	br, bc := len(b), len(b[0])
	if ac != br {
		return nil, errors.New("codec: mismatched matrix dimensions")
	}
	out := newMatrix(ar, bc)
	for i := 0; i < ar; i++ {
		for k := 0; k < ac; k++ {
			if a[i][k] == 0 {
				continue
			}
			for j := 0; j < bc; j++ {
				out[i][j] ^= galois.Mul(a[i][k], b[k][j])
			}
		}
	}
	return out, nil
}

// invert returns the inverse of a square matrix over GF(2^8) using Gauss-Jordan
// elimination with partial pivoting. It returns ErrMatrixNotInvertible when the
// matrix is singular.
func Invert(a matrix) (matrix, error) {
	n := len(a)
	if n == 0 || len(a[0]) != n {
		return nil, errors.New("codec: cannot invert non-square matrix")
	}
	// Augment [a | I].
	aug := newMatrix(n, 2*n)
	for i := 0; i < n; i++ {
		copy(aug[i][:n], a[i])
		aug[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		// Find a pivot in this column at or below the current row.
		pivot := -1
		for row := col; row < n; row++ {
			if aug[row][col] != 0 {
				pivot = row
				break
			}
		}
		if pivot == -1 {
			return nil, ErrMatrixNotInvertible
		}
		aug[pivot], aug[col] = aug[col], aug[pivot]
		// Scale the pivot row so the diagonal entry becomes 1.
		invPivot, err := galois.Inverse(aug[col][col])
		if err != nil {
			return nil, ErrMatrixNotInvertible
		}
		for j := 0; j < 2*n; j++ {
			aug[col][j] = galois.Mul(invPivot, aug[col][j])
		}
		// Eliminate the pivot column from every other row.
		for row := 0; row < n; row++ {
			if row == col || aug[row][col] == 0 {
				continue
			}
			factor := aug[row][col]
			for j := 0; j < 2*n; j++ {
				aug[row][j] ^= galois.Mul(factor, aug[col][j])
			}
		}
	}
	out := newMatrix(n, n)
	for i := 0; i < n; i++ {
		copy(out[i], aug[i][n:])
	}
	return out, nil
}

// buildEncodingMatrix builds the parity x data encoding matrix m such that
// parity shard p = sum_c m[p][c] * data shard c. It is derived from a
// Vandermonde matrix by inverting its top (data x data) block.
func buildEncodingMatrix(dataShards, parityShards int) (matrix, error) {
	total := dataShards + parityShards
	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvalidShardCount
	}
	if total > maxTotalShards {
		return nil, ErrInvalidShardCount
	}
	vm := vandermonde(total, dataShards)
	top := subMatrix(vm, 0, 0, dataShards, dataShards)
	invTop, err := Invert(top)
	if err != nil {
		return nil, err
	}
	bottom := subMatrix(vm, dataShards, 0, parityShards, dataShards)
	m, err := multiply(bottom, invTop)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// BuildCodeMatrix builds the full total x data code matrix C where the first
// dataShards rows are the identity (so data shards pass through unchanged) and
// the remaining parity rows are the encoding matrix. Any dataShards rows of C
// are linearly independent, which is what makes reconstruction possible.
func BuildCodeMatrix(dataShards, parityShards int) (matrix, error) {
	m, err := buildEncodingMatrix(dataShards, parityShards)
	if err != nil {
		return nil, err
	}
	total := dataShards + parityShards
	C := newMatrix(total, dataShards)
	for i := 0; i < dataShards; i++ {
		C[i][i] = 1
	}
	for i := 0; i < parityShards; i++ {
		copy(C[dataShards+i], m[i])
	}
	return C, nil
}
