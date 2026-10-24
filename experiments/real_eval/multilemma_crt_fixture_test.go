package realeval

import (
	"math/big"
	"testing"
)

// These are PUBLIC DEVELOPMENT FIXTURES, not unseen LLM questions.
// They check a two-premise CRT certificate; they do not establish
// nontrivial Erdős–Straus proof composition or a CSR retrieval advantage.
type crtDevFixture struct {
	m1, r1 int64
	m2, r2 int64
}

func crtDevDerive(f crtDevFixture) (*big.Int, *big.Int, bool) {
	if f.m1 <= 1 || f.m2 <= 1 ||
		f.r1 < 0 || f.r1 >= f.m1 ||
		f.r2 < 0 || f.r2 >= f.m2 {
		return nil, nil, false
	}
	m1, m2 := big.NewInt(f.m1), big.NewInt(f.m2)
	g := new(big.Int).GCD(nil, nil, m1, m2)
	if g.Cmp(big.NewInt(1)) != 0 {
		return nil, nil, false
	}
	inverse := new(big.Int).ModInverse(m1, m2)
	if inverse == nil {
		return nil, nil, false
	}
	k := new(big.Int).Sub(big.NewInt(f.r2), big.NewInt(f.r1))
	k.Mul(k, inverse)
	k.Mod(k, m2)
	M := new(big.Int).Mul(m1, m2)
	R := new(big.Int).Mul(k, m1)
	R.Add(R, big.NewInt(f.r1))
	R.Mod(R, M)
	return R, M, true
}

func crtDevVerify(f crtDevFixture, R, M *big.Int) bool {
	if R == nil || M == nil || f.m1 <= 1 || f.m2 <= 1 {
		return false
	}
	if new(big.Int).GCD(nil, nil, big.NewInt(f.m1), big.NewInt(f.m2)).Cmp(big.NewInt(1)) != 0 {
		return false
	}
	if M.Cmp(new(big.Int).Mul(big.NewInt(f.m1), big.NewInt(f.m2))) != 0 ||
		R.Sign() < 0 || R.Cmp(M) >= 0 {
		return false
	}
	return new(big.Int).Mod(R, big.NewInt(f.m1)).Int64() == f.r1 &&
		new(big.Int).Mod(R, big.NewInt(f.m2)).Int64() == f.r2
}

// Intentionally independent exhaustive oracle for these small fixtures.
// Count candidate residue classes modulo M after retaining the chosen premises.
func crtDevCountClasses(f crtDevFixture, requireFirst, requireSecond bool) (int64, int64) {
	M := f.m1 * f.m2 // these small fixture moduli are far below int64 overflow
	count, witness := int64(0), int64(-1)
	for n := int64(0); n < M; n++ {
		if requireFirst && n%f.m1 != f.r1 {
			continue
		}
		if requireSecond && n%f.m2 != f.r2 {
			continue
		}
		count++
		witness = n
	}
	return count, witness
}

func TestMultiLemmaCRTDevelopmentCertificates(t *testing.T) {
	fixtures := []crtDevFixture{
		{8, 5, 9, 7}, {5, 3, 7, 4}, {7, 2, 11, 3},
		{11, 6, 13, 8}, {13, 5, 17, 12}, {9, 4, 16, 13},
		{16, 9, 25, 21}, {17, 7, 19, 8}, {19, 13, 23, 18},
		{23, 10, 29, 22}, {25, 6, 27, 14}, {27, 11, 32, 17},
	}
	for i, f := range fixtures {
		R, M, ok := crtDevDerive(f)
		if !ok || !crtDevVerify(f, R, M) {
			t.Fatalf("fixture %d: invalid CRT certificate for %+v", i, f)
		}
		count, witness := crtDevCountClasses(f, true, true)
		if count != 1 || R.Int64() != witness {
			t.Fatalf("fixture %d: independent enumeration disagrees with CRT: count=%d, got=%s, brute=%d", i, count, R, witness)
		}
		onlyFirst, _ := crtDevCountClasses(f, true, false)
		onlySecond, _ := crtDevCountClasses(f, false, true)
		if onlyFirst != f.m2 || onlySecond != f.m1 {
			t.Fatalf("fixture %d: removing either premise must make the unique class underdetermined: %d, %d", i, onlyFirst, onlySecond)
		}
		badR := new(big.Int).Add(R, big.NewInt(1))
		if crtDevVerify(f, badR, M) {
			t.Fatalf("fixture %d: accepted incorrect CRT residue", i)
		}
		badM := new(big.Int).Add(M, big.NewInt(1))
		if crtDevVerify(f, R, badM) {
			t.Fatalf("fixture %d: accepted incorrect CRT modulus", i)
		}
	}
}

func TestMultiLemmaCRTRejectsInvalidFixtures(t *testing.T) {
	for _, f := range []crtDevFixture{
		{6, 1, 10, 2}, // inconsistent noncoprime system
		{4, 1, 6, 1},  // noncoprime (a generalized CRT could still be used)
		{1, 0, 7, 3},  // a trivial premise does not establish two-lemma minimality
		{5, 5, 7, 1},  // noncanonical residue
		{5, -1, 7, 1},
	} {
		if _, _, ok := crtDevDerive(f); ok {
			t.Fatalf("CRT minimality fixture should have been rejected: %+v", f)
		}
	}
}
