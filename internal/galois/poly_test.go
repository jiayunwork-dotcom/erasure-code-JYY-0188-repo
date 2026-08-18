package galois

import (
	"testing"
)

func TestPolyMulIdentity(t *testing.T) {
	// Multiplying by [1] (the polynomial "1") should be identity.
	p := []byte{0x03, 0x07, 0x0A}
	one := []byte{1}
	result := PolyMul(p, one)
	if len(result) != len(p) {
		t.Fatalf("expected length %d, got %d", len(p), len(result))
	}
	for i := range p {
		if result[i] != p[i] {
			t.Fatalf("mismatch at %d: got %02x, want %02x", i, result[i], p[i])
		}
	}
}

func TestPolyAddSelf(t *testing.T) {
	// p + p = 0 in GF(2^8).
	p := []byte{0x12, 0x34, 0x56}
	result := PolyAdd(p, p)
	for i := range result {
		if result[i] != 0 {
			t.Fatalf("expected zero at %d, got %02x", i, result[i])
		}
	}
}

func TestPolyDivMod(t *testing.T) {
	// (a * b) / b should give quotient = a, remainder = 0.
	a := []byte{0x05, 0x03}       // 5 + 3x
	b := []byte{0x01, 0x01, 0x01} // 1 + x + x^2
	prod := PolyMul(a, b)
	q, rem := PolyDivMod(prod, b)
	// Quotient should equal a.
	if len(q) != len(a) {
		t.Fatalf("quotient length %d, want %d", len(q), len(a))
	}
	for i := range a {
		if q[i] != a[i] {
			t.Fatalf("quotient[%d] = %02x, want %02x", i, q[i], a[i])
		}
	}
	// Remainder should be zero.
	allZero := true
	for _, r := range rem {
		if r != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Fatalf("expected zero remainder, got %v", rem)
	}
}

func TestGeneratorPolyDegree(t *testing.T) {
	n := 4
	g := GeneratorPoly(n)
	deg := PolyDeg(g)
	if deg != n {
		t.Fatalf("expected degree %d, got %d", n, deg)
	}
}

func TestSyndromeEvalCodeword(t *testing.T) {
	// For a valid codeword (product of generator), all syndromes should be 0.
	n := 3
	g := GeneratorPoly(n)
	// Create a codeword by multiplying a message polynomial by g.
	msg := []byte{0x05, 0x02}
	codeword := PolyMul(msg, g)
	synd := SyndromeEval(codeword, n)
	for i, s := range synd {
		if s != 0 {
			t.Fatalf("syndrome[%d] = %02x, want 0", i, s)
		}
	}
}

func TestPolyScale(t *testing.T) {
	p := []byte{0x01, 0x02, 0x03}
	scaled := PolyScale(p, 0x02)
	for i := range p {
		want := Mul(p[i], 0x02)
		if scaled[i] != want {
			t.Fatalf("scaled[%d] = %02x, want %02x", i, scaled[i], want)
		}
	}
}
