// Package galois provides arithmetic over the binary Galois field GF(2^8).
//
// The field is defined by the primitive polynomial 0x11D (x^8 + x^4 + x^3 + x^2
// + 1). Elements are represented as bytes. Multiplication is accelerated with
// precomputed exponent/logarithm tables and a full 256x256 multiplication
// table.
package galois

import (
	"errors"
	"fmt"
)

// Size is the number of elements in GF(2^8).
const Size = 256

// PrimePoly is the primitive polynomial 0x11D used to define GF(2^8).
const PrimePoly = 0x11D

// expTableLen is the length of the exponent table. It is double the field size
// so that Exp[i+j] can be looked up directly without a modulo for any pair of
// logarithms in [0, 254].
const expTableLen = 512

// Exp is the precomputed exponent table. Exp[i] == alpha^i, with period 255.
// Indices in [255, 511] repeat [0, 254] so additions of logarithms never
// overflow the table.
var Exp [expTableLen]byte

// Log is the precomputed discrete-logarithm table. Log[Exp[i]] == i for
// i in [0, 254]. Log[0] is undefined and left as 0.
var Log [Size]byte

// MulTable is the full 256x256 multiplication table: MulTable[a][b] == Mul(a, b).
// It is built once during initialization for fast repeated multiplications.
var MulTable [Size][Size]byte

// ErrDivideByZero is returned by Div and Inverse when the operand is zero.
var ErrDivideByZero = errors.New("galois: divide by zero")

// ErrInverseOfZero is returned by Inverse when the argument is zero.
var ErrInverseOfZero = errors.New("galois: inverse of zero")

func init() {
	buildTables()
}

// buildTables populates Exp, Log, and MulTable.
func buildTables() {
	x := 1
	for i := 0; i < 255; i++ {
		Exp[i] = byte(x)
		Log[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= PrimePoly
		}
	}
	// Mirror the first 255 entries to cover logarithm sums up to 508.
	for i := 255; i < expTableLen; i++ {
		Exp[i] = Exp[i-255]
	}
	// MulTable[a][b] == Mul(a, b).
	for a := 0; a < Size; a++ {
		for b := 0; b < Size; b++ {
			MulTable[a][b] = mulRaw(byte(a), byte(b))
		}
	}
}

// Add returns the field sum of a and b. Addition in GF(2^8) is XOR.
func Add(a, b byte) byte {
	return a ^ b
}

// mulRaw computes a*b without consulting the table. It is used to build the
// table itself.
func mulRaw(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return Exp[int(Log[a])+int(Log[b])]
}

// Mul returns the field product of a and b.
func Mul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return Exp[int(Log[a])+int(Log[b])]
}

// MulFast returns the field product of a and b using the precomputed table.
func MulFast(a, b byte) byte {
	return MulTable[a][b]
}

// Div returns the field quotient a/b. It returns ErrDivideByZero when b == 0.
func Div(a, b byte) (byte, error) {
	if b == 0 {
		return 0, ErrDivideByZero
	}
	if a == 0 {
		return 0, nil
	}
	diff := int(Log[a]) - int(Log[b])
	if diff < 0 {
		diff += 255
	}
	return Exp[diff], nil
}

// Inverse returns the multiplicative inverse of a in the field. It returns
// ErrInverseOfZero when a == 0.
func Inverse(a byte) (byte, error) {
	if a == 0 {
		return 0, ErrInverseOfZero
	}
	return Exp[255-int(Log[a])], nil
}

// Pow returns a raised to the power y in the field. By convention,
// Pow(x, 0) == 1 and Pow(0, 0) == 1; Pow(0, y) == 0 for y > 0.
func Pow(a byte, y int) byte {
	if y == 0 {
		return 1
	}
	if a == 0 {
		return 0
	}
	return Exp[(int(Log[a])*y)%255]
}

// MulSlice computes dst[i] ^= Mul(k, src[i]) for every index i. It is used to
// accumulate scaled shard vectors during encoding and reconstruction. The
// source and destination slices must have equal length.
func MulSlice(k byte, src, dst []byte) {
	if k == 0 {
		return
	}
	for i := range src {
		dst[i] ^= Mul(k, src[i])
	}
}

// MulSliceAdd is an alias of MulSlice retained for clarity at call sites that
// accumulate into an existing accumulator. It writes dst[i] ^= Mul(k, src[i]).
func MulSliceAdd(k byte, src, dst []byte) {
	MulSlice(k, src, dst)
}

// MulSliceTable computes dst[i] ^= MulTable[k][src[i]] for every index i using
// the precomputed table. It is the table-accelerated variant of MulSlice.
func MulSliceTable(k byte, src, dst []byte) {
	if k == 0 {
		return
	}
	row := MulTable[k]
	for i := range src {
		dst[i] ^= row[src[i]]
	}
}

// Equal reports whether two byte slices are element-wise equal in the field.
// It is a trivial helper used by tests and callers comparing shard vectors.
func Equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Sum returns the XOR (field sum) of all elements in xs. The sum of an empty
// slice is 0.
func Sum(xs []byte) byte {
	var s byte
	for _, x := range xs {
		s ^= x
	}
	return s
}

// PolyEval evaluates the polynomial p at x in GF(2^8), where p[0] is the
// constant term, p[1] the coefficient of x, and so on. Horner's method is used:
// p(x) = p[0] + x*(p[1] + x*(p[2] + ...)).
func PolyEval(p []byte, x byte) byte {
	var acc byte
	for i := len(p) - 1; i >= 0; i-- {
		acc = Add(p[i], Mul(acc, x))
	}
	return acc
}

// SelfCheck verifies the internal Exp, Log, and MulTable are mutually consistent
// and that the defining field identities hold. It returns a descriptive error on
// the first inconsistency found, or nil when everything checks out.
func SelfCheck() error {
	for x := 1; x < Size; x++ {
		if Exp[int(Log[byte(x)])] != byte(x) {
			return fmt.Errorf("galois: Exp(Log(%d)) != %d", x, x)
		}
		if Pow(byte(x), 255) != 1 {
			return fmt.Errorf("galois: Pow(%d,255) != 1", x)
		}
		inv, err := Inverse(byte(x))
		if err != nil {
			return fmt.Errorf("galois: Inverse(%d): %w", x, err)
		}
		if Mul(byte(x), inv) != 1 {
			return fmt.Errorf("galois: %d*Inverse(%d) != 1", x, x)
		}
		for y := 0; y < Size; y++ {
			if Mul(byte(x), byte(y)) != MulTable[x][y] {
				return fmt.Errorf("galois: Mul(%d,%d) != MulTable", x, y)
			}
		}
	}
	return nil
}
