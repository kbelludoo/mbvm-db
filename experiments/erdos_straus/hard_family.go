package erdosstraus

import (
	"fmt"
	"math/big"
)

var (
	threeBig  = big.NewInt(3)
	twelveBig = big.NewInt(12)
	fortyEight = big.NewInt(48)
)

// HardClassParametricFamily instantiates the closed polynomial solution:
// x(n) = (n + 3)/4
// y(n) = (n(n+3) + 4p) / 12
// z(n) = n(n+3)(n(n+3) + 4p) / (48p)
// for any prime p = 2 mod 3 and n = -3 mod (4p) with n = 1 mod 3.
func HardClassParametricFamily(id string, p int64, hardResidue840 int64, combinedMod int64, combinedRes int64) *Rule {
	proof := fmt.Sprintf(
		"Closed Rational Identity in Q(n):\n"+
			"Let x = (n+3)/4. Then 4/n - 1/x = 3/(nx).\n"+
			"Setting (3y - nx)(3z - nx) = (nx)^2 with divisor u = p gives:\n"+
			"y(n) = (n(n+3) + 4p) / 12\n"+
			"z(n) = n(n+3)[n(n+3) + 4p] / (48p)\n"+
			"Identically, 1/x + 1/y + 1/z = 4/n.\n"+
			"Integrality Certificate:\n"+
			"For n = %d mod %d (which projects to %d mod 840 in the hard class):\n"+
			"1. n = 1 mod 4 => 4 | (n+3) => x is integer.\n"+
			"2. n = 1 mod 3 and p = 2 mod 3 => n(n+3) + 4p = 1(1) + 4(2) = 9 = 0 mod 3, and 4 | (n(n+3)+4p) => y is integer.\n"+
			"3. p | (n+3) => p^2 | n(n+3)[n(n+3)+4p], and 48 | numerator => z is integer.\n"+
			"Crucially, n = 1 mod 24, so this family is strictly disjoint from all elementary classes (n even, n=3 mod 4, n=2 mod 3, n=5 mod 8).",
		combinedRes, combinedMod, hardResidue840)

	pBig := big.NewInt(p)
	fourP := new(big.Int).Mul(big.NewInt(4), pBig)
	fortyEightP := new(big.Int).Mul(fortyEight, pBig)

	return &Rule{
		ID:           id,
		Name:         fmt.Sprintf("Hard Residue %d mod 840 Family (p=%d)", hardResidue840, p),
		Hypotheses:   []string{fmt.Sprintf("n = %d mod %d", combinedRes, combinedMod)},
		Conclusion:   fmt.Sprintf("Closed polynomial solution: x=(n+3)/4, y=(n(n+3)+%d)/12, z=n(n+3)[n(n+3)+%d]/%d", 4*p, 4*p, 48*p),
		Modulus:      combinedMod,
		Residues:     []int64{combinedRes},
		BibSource:    "Mordell (1967) divisor specialization with closed polynomial formulation via MBVM-DB",
		ProofText:    proof,
		Status:       StatusVerified,
		TokenWeight:  85,
		SolveFunc: func(n *big.Int) (*Solution, error) {
			// x = (n + 3)/4
			nPlus3 := new(big.Int).Add(n, threeBig)
			if new(big.Int).Mod(nPlus3, four).Sign() != 0 {
				return nil, fmt.Errorf("n+3 not divisible by 4: %w", ErrInvalidInput)
			}
			x := new(big.Int).Div(nPlus3, four)

			// n(n+3)
			nNplus3 := new(big.Int).Mul(n, nPlus3)

			// y = (n(n+3) + 4p) / 12
			yNum := new(big.Int).Add(nNplus3, fourP)
			if new(big.Int).Mod(yNum, twelveBig).Sign() != 0 {
				return nil, fmt.Errorf("y numerator not divisible by 12: %w", ErrInvalidInput)
			}
			y := new(big.Int).Div(yNum, twelveBig)

			// z = n(n+3)[n(n+3) + 4p] / (48p)
			zNum := new(big.Int).Mul(nNplus3, yNum)
			if new(big.Int).Mod(zNum, fortyEightP).Sign() != 0 {
				return nil, fmt.Errorf("z numerator not divisible by 48p: %w", ErrInvalidInput)
			}
			z := new(big.Int).Div(zNum, fortyEightP)

			sol := &Solution{X: x, Y: y, Z: z}
			if err := VerifySolution(n, sol); err != nil {
				return nil, fmt.Errorf("hard family verification failed: %w", err)
			}
			return sol, nil
		},
	}
}

// HardClasses840ParametricRules constructs closed polynomial families for all 6 hard residue classes mod 840.
func HardClasses840ParametricRules() []*Rule {
	return []*Rule{
		// 1. n = 1 mod 840 with p = 11 => n = 8401 mod 9240
		HardClassParametricFamily("RULE-HARD-840-RES1-P11", 11, 1, 9240, 8401),

		// 2. n = 121 mod 840 with p = 11 => n = 1801 mod 9240
		HardClassParametricFamily("RULE-HARD-840-RES121-P11", 11, 121, 9240, 1801),

		// 3. n = 169 mod 840 with p = 11 => n = 1009 mod 9240
		HardClassParametricFamily("RULE-HARD-840-RES169-P11", 11, 169, 9240, 1009),

		// 4. n = 289 mod 840 with p = 11 => n = 3649 mod 9240
		HardClassParametricFamily("RULE-HARD-840-RES289-P11", 11, 289, 9240, 3649),

		// 5. n = 361 mod 840 with p = 11 => n = 7081 mod 9240
		HardClassParametricFamily("RULE-HARD-840-RES361-P11", 11, 361, 9240, 7081),

		// 6. n = 529 mod 840 with p = 11 => n = 8929 mod 9240
		HardClassParametricFamily("RULE-HARD-840-RES529-P11", 11, 529, 9240, 8929),

		// 7. n = 1 mod 840 with p = 17 => n = 11761 mod 14280
		HardClassParametricFamily("RULE-HARD-840-RES1-P17", 17, 1, 14280, 11761),

		// 8. n = 1 mod 840 with p = 23 => n = 12601 mod 19320
		HardClassParametricFamily("RULE-HARD-840-RES1-P23", 23, 1, 19320, 12601),
	}
}
