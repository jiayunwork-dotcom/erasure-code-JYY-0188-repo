package galois

// PolyMul multiplies two polynomials over GF(2^8). The result polynomial has
// degree len(a)-1 + len(b)-1. Coefficients are indexed from constant term
// upward: p[0] is the x^0 term.
func PolyMul(a, b []byte) []byte {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	result := make([]byte, len(a)+len(b)-1)
	for i, ai := range a {
		if ai == 0 {
			continue
		}
		for j, bj := range b {
			if bj == 0 {
				continue
			}
			result[i+j] ^= Mul(ai, bj)
		}
	}
	return result
}

// PolyAdd adds two polynomials over GF(2^8). The result has length
// max(len(a), len(b)).
func PolyAdd(a, b []byte) []byte {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	result := make([]byte, n)
	for i := range a {
		result[i] ^= a[i]
	}
	for i := range b {
		result[i] ^= b[i]
	}
	return result
}

// PolyScale multiplies every coefficient of p by scalar k.
func PolyScale(p []byte, k byte) []byte {
	result := make([]byte, len(p))
	for i, c := range p {
		result[i] = Mul(c, k)
	}
	return result
}

// PolyDivMod divides polynomial a by polynomial b, returning quotient and
// remainder. Returns (nil, nil) if b is zero-length (division by zero).
func PolyDivMod(a, b []byte) (quotient, remainder []byte) {
	if len(b) == 0 {
		return nil, nil
	}
	// Trim leading zeros from b (high degree end).
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	if len(b) == 0 {
		return nil, nil
	}
	rem := make([]byte, len(a))
	copy(rem, a)
	degA := len(a) - 1
	degB := len(b) - 1
	if degA < degB {
		return []byte{0}, rem
	}
	quotient = make([]byte, degA-degB+1)
	leadInv, _ := Inverse(b[degB])
	for i := degA; i >= degB; i-- {
		if rem[i] == 0 {
			continue
		}
		coeff := Mul(rem[i], leadInv)
		quotient[i-degB] = coeff
		for j := 0; j <= degB; j++ {
			rem[i-degB+j] ^= Mul(coeff, b[j])
		}
	}
	// Trim remainder.
	for len(rem) > 1 && rem[len(rem)-1] == 0 {
		rem = rem[:len(rem)-1]
	}
	return quotient, rem
}

// PolyDeg returns the degree of the polynomial (index of highest non-zero
// coefficient). Returns -1 for the zero polynomial.
func PolyDeg(p []byte) int {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] != 0 {
			return i
		}
	}
	return -1
}

// GeneratorPoly builds the generator polynomial of degree n:
//
//	g(x) = (x - alpha^0)(x - alpha^1)...(x - alpha^(n-1))
//
// in GF(2^8). This is used in BCH/RS encoding as the divisor polynomial.
func GeneratorPoly(n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		root := Pow(2, i) // alpha^i where alpha=2 is a primitive element
		factor := []byte{root, 1}
		g = PolyMul(g, factor)
	}
	return g
}

// SyndromeEval evaluates the polynomial p at alpha^0, alpha^1, ..., alpha^(n-1)
// and returns the n syndrome values. A codeword has all-zero syndromes.
func SyndromeEval(p []byte, n int) []byte {
	synd := make([]byte, n)
	for i := 0; i < n; i++ {
		synd[i] = PolyEval(p, Pow(2, i))
	}
	return synd
}
