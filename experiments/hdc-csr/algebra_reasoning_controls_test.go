package hdccsr

import (
	"math"
	"math/rand"
	"testing"
)

// This laboratory separates learned OOD arithmetic from built-in VSA algebra.
// It does not claim that XOR binding is addition or that Unbind solves math.

type arithmeticPair struct{ a, b int }

type affineArithmeticNet struct{ wa, wb, bias float64 }

func (m affineArithmeticNet) predict(a, b int) float64 {
	return m.wa*float64(a) + m.wb*float64(b) + m.bias
}

// Training uses labeled examples ONLY from [-10,10] x [-10,10].
// No arithmetic rule or OOD test answers are supplied to this network.
func trainAffineArithmetic(train []arithmeticPair, target func(int, int) float64) affineArithmeticNet {
	m := affineArithmeticNet{wa: 0.17, wb: -0.23, bias: 0.13}
	const rate = 0.002
	for epoch := 0; epoch < 6000; epoch++ {
		ga, gb, gc := 0.0, 0.0, 0.0
		for _, p := range train {
			err := m.predict(p.a, p.b) - target(p.a, p.b)
			ga += 2 * err * float64(p.a)
			gb += 2 * err * float64(p.b)
			gc += 2 * err
		}
		scale := rate / float64(len(train))
		m.wa -= scale * ga
		m.wb -= scale * gb
		m.bias -= scale * gc
	}
	return m
}

func arithmeticTrainHoldout() ([]arithmeticPair, []arithmeticPair) {
	all := make([]arithmeticPair, 0, 441)
	for a := -10; a <= 10; a++ {
		for b := -10; b <= 10; b++ {
			all = append(all, arithmeticPair{a, b})
		}
	}
	rng := rand.New(rand.NewSource(2026))
	rng.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return all[:353], all[353:]
}

func arithmeticOOD() []arithmeticPair {
	pairs := make([]arithmeticPair, 0, 400)
	for a := 11; a <= 30; a++ {
		for b := 11; b <= 30; b++ {
			pairs = append(pairs, arithmeticPair{a, b})
		}
	}
	return pairs
}

func roundedAccuracy(m affineArithmeticNet, samples []arithmeticPair, target func(int, int) float64) float64 {
	correct := 0
	for _, p := range samples {
		if math.Round(m.predict(p.a, p.b)) == target(p.a, p.b) {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func TestAlgebraLabLearnedArithmeticOOD(t *testing.T) {
	train, holdout := arithmeticTrainHoldout()
	ood := arithmeticOOD()
	addition := func(a, b int) float64 { return float64(a + b) }
	multiplication := func(a, b int) float64 { return float64(a * b) }
	addModel := trainAffineArithmetic(train, addition)
	mulModel := trainAffineArithmetic(train, multiplication)
	addHoldout := roundedAccuracy(addModel, holdout, addition)
	addOOD := roundedAccuracy(addModel, ood, addition)
	mulHoldout := roundedAccuracy(mulModel, holdout, multiplication)
	mulOOD := roundedAccuracy(mulModel, ood, multiplication)
	t.Logf("linear learned (3 parameters) ADD: holdout[-10,10]=%.2f%% OOD[11,30]=%.2f%% weights=(%.6f,%.6f,%.6f)", 100*addHoldout, 100*addOOD, addModel.wa, addModel.wb, addModel.bias)
	t.Logf("linear learned (3 parameters) MULTIPLY: holdout[-10,10]=%.2f%% OOD[11,30]=%.2f%%", 100*mulHoldout, 100*mulOOD)
	// An affine network can learn addition and extrapolate beyond train range.
	if addHoldout < 1 || addOOD < 1 {
		t.Fatalf("linear addition did not extrapolate: holdout=%g OOD=%g", addHoldout, addOOD)
	}
	if mulOOD > 0.5 {
		t.Fatalf("unexpected multiplication extrapolation with no interaction feature: %g", mulOOD)
	}
}

// Reference: the exact integer rule is GIVEN; it is not a learned model.
func TestAlgebraLabExplicitRuleReference(t *testing.T) {
	for _, p := range arithmeticOOD() {
		if p.a+p.b != p.b+p.a {
			t.Fatal("integer addition commutativity broken")
		}
		if (p.a+p.b)+7 != p.a+(p.b+7) {
			t.Fatal("integer addition associativity broken")
		}
		if p.a*(p.b+7) != p.a*p.b+p.a*7 {
			t.Fatal("integer distributivity broken")
		}
	}
}

func TestAlgebraLabHDCAxiomsNotArithmetic(t *testing.T) {
	rng := rand.New(rand.NewSource(2026))
	var zero Hypervector
	for i := 0; i < 50; i++ {
		a := GenerateRandomHypervector(rng)
		b := GenerateRandomHypervector(rng)
		c := GenerateRandomHypervector(rng)
		if Bind(a, b) != Bind(b, a) {
			t.Fatal("XOR not commutative")
		}
		if Bind(Bind(a, b), c) != Bind(a, Bind(b, c)) {
			t.Fatal("XOR not associative")
		}
		if Bind(a, zero) != a || Bind(a, a) != zero {
			t.Fatal("XOR identity/inverse broken")
		}
		if Unbind(Bind(a, b), a) != b {
			t.Fatal("exact unbinding failed")
		}
		if HammingDistance(Bind(a, c), Bind(b, c)) != HammingDistance(a, b) {
			t.Fatal("distance preservation broken")
		}
	}
	// The two operations differ even when both are commutative.
	if (1 ^ 1) == (1 + 1) {
		t.Fatal("XOR must not be mistaken for natural addition")
	}
	t.Log("HDC XOR axioms: 100% by construction; no labeled arithmetic problem was learned by HDC")
}
