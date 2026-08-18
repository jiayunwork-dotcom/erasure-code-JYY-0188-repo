// Package cauchy provides Cauchy matrix based Reed-Solomon encoding. A Cauchy
// matrix C[i][j] = 1/(x_i + y_j) over GF(2^8) is an alternative to the
// Vandermonde approach used in the codec package. Any square sub-matrix of a
// Cauchy matrix is guaranteed invertible, which gives optimal reconstruction
// properties.
package cauchy

import (
	"errors"

	"erasure-code/internal/galois"
)

// ErrInvalidParams is returned when data or parity shard counts are non-positive
// or exceed the field capacity.
var ErrInvalidParams = errors.New("cauchy: invalid parameters")

// ErrTooManyShards is returned when data+parity exceeds 127 (the limit for
// constructing distinct x and y sets within GF(2^8)).
var ErrTooManyShards = errors.New("cauchy: too many shards for GF(2^8)")

// ErrNotEnoughShards is returned when fewer than dataShards shards are available
// for reconstruction.
var ErrNotEnoughShards = errors.New("cauchy: not enough shards to reconstruct")

// ErrShardSizeMismatch is returned when shards have inconsistent lengths.
var ErrShardSizeMismatch = errors.New("cauchy: shard size mismatch")

// maxHalfField is the largest allowed data or parity count. The Cauchy matrix
// construction requires distinct x_i in [0, data-1] and y_j in [data, data+parity-1]
// all mapped to distinct GF(2^8) elements; keeping total ≤ 127 ensures no collision.
const maxHalfField = 127

// Matrix is a row-major matrix over GF(2^8).
type Matrix [][]byte

// NewMatrix allocates a rows x cols zero matrix.
func NewMatrix(rows, cols int) Matrix {
	m := make(Matrix, rows)
	for r := range m {
		m[r] = make([]byte, cols)
	}
	return m
}

// Rows returns the number of rows.
func (m Matrix) Rows() int { return len(m) }

// Cols returns the number of columns. Returns 0 for an empty matrix.
func (m Matrix) Cols() int {
	if len(m) == 0 {
		return 0
	}
	return len(m[0])
}

// Get returns element at (r, c).
func (m Matrix) Get(r, c int) byte { return m[r][c] }

// Set sets element at (r, c).
func (m Matrix) Set(r, c int, v byte) { m[r][c] = v }

// CauchyMatrix builds a parity x data Cauchy matrix. Element (i, j) is
// 1/(x_i XOR y_j) where x_i = i and y_j = data + j (all distinct in GF(2^8)).
func CauchyMatrix(data, parity int) (Matrix, error) {
	if data <= 0 || parity <= 0 {
		return nil, ErrInvalidParams
	}
	total := data + parity
	if total > maxHalfField {
		return nil, ErrTooManyShards
	}
	m := NewMatrix(parity, data)
	for i := 0; i < parity; i++ {
		xi := byte(data + i)
		for j := 0; j < data; j++ {
			yj := byte(j)
			// x_i XOR y_j is guaranteed non-zero when x and y sets are disjoint.
			sum := galois.Add(xi, yj)
			inv, _ := galois.Inverse(sum)
			m[i][j] = inv
		}
	}
	return m, nil
}

// FullMatrix builds the (data+parity) x data code matrix where the top data rows
// are the identity and the bottom parity rows are the Cauchy encoding matrix.
func FullMatrix(data, parity int) (Matrix, error) {
	cm, err := CauchyMatrix(data, parity)
	if err != nil {
		return nil, err
	}
	total := data + parity
	full := NewMatrix(total, data)
	for i := 0; i < data; i++ {
		full[i][i] = 1
	}
	for i := 0; i < parity; i++ {
		copy(full[data+i], cm[i])
	}
	return full, nil
}

// SubMatrix extracts a sub x cols matrix from selected rows.
func SubMatrix(m Matrix, rows []int) Matrix {
	cols := m.Cols()
	sub := NewMatrix(len(rows), cols)
	for i, r := range rows {
		copy(sub[i], m[r])
	}
	return sub
}

// Invert returns the inverse of a square matrix over GF(2^8) using Gauss-Jordan
// elimination. Returns an error if the matrix is singular.
func Invert(m Matrix) (Matrix, error) {
	n := m.Rows()
	if n == 0 || m.Cols() != n {
		return nil, errors.New("cauchy: cannot invert non-square matrix")
	}
	// Build augmented [m | I].
	aug := NewMatrix(n, 2*n)
	for i := 0; i < n; i++ {
		copy(aug[i][:n], m[i])
		aug[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		pivot := -1
		for row := col; row < n; row++ {
			if aug[row][col] != 0 {
				pivot = row
				break
			}
		}
		if pivot == -1 {
			return nil, errors.New("cauchy: matrix is singular")
		}
		aug[pivot], aug[col] = aug[col], aug[pivot]
		invPivot, _ := galois.Inverse(aug[col][col])
		for j := 0; j < 2*n; j++ {
			aug[col][j] = galois.Mul(invPivot, aug[col][j])
		}
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
	out := NewMatrix(n, n)
	for i := 0; i < n; i++ {
		copy(out[i], aug[i][n:])
	}
	return out, nil
}
