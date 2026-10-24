package erdosstraus

import (
	"fmt"
	"math/big"
)

// AdditivePartitionFamily creates a parametric Rule based on:
// x = (n + A)/4, y = nx/u, z = nx/v where A = u + v.
func AdditivePartitionFamily(id, name string, A, u, v int64, mod int64, res int64) *Rule {
	proof := fmt.Sprintf(
		"Let x = (n + %d)/4. Then 4/n - 1/x = (4x - n)/(nx) = %d/(nx). "+
			"Since %d = %d + %d, %d/(nx) = %d/(nx) + %d/(nx) = 1/(nx/%d) + 1/(nx/%d). "+
			"For n = %d mod %d, u=%d and v=%d divide nx, ensuring y and z are positive integers.",
		A, A, A, u, v, A, u, v, u, v, res, mod, u, v)

	return &Rule{
		ID:         id,
		Name:       name,
		Hypotheses: []string{fmt.Sprintf("n = %d mod %d", res, mod)},
		Conclusion: fmt.Sprintf("4/n = 1/x + 1/(nx/%d) + 1/(nx/%d) with x=(n+%d)/4", u, v, A),
		Modulus:    mod,
		Residues:   []int64{res},
		BibSource:  "Additive Partition Lemma derived via MBVM-DB Composer",
		ProofText:  proof,
		Status:     StatusVerified,
		TokenWeight: 65,
		SolveFunc: func(n *big.Int) (*Solution, error) {
			// x = (n + A)/4
			nPlusA := new(big.Int).Add(n, big.NewInt(A))
			if new(big.Int).Mod(nPlusA, four).Sign() != 0 {
				return nil, fmt.Errorf("4 does not divide n+%d: %w", A, ErrInvalidInput)
			}
			x := new(big.Int).Div(nPlusA, four)

			nx := new(big.Int).Mul(n, x)
			uBig := big.NewInt(u)
			vBig := big.NewInt(v)

			if new(big.Int).Mod(nx, uBig).Sign() != 0 || new(big.Int).Mod(nx, vBig).Sign() != 0 {
				return nil, fmt.Errorf("denominators not integral: u=%d or v=%d does not divide nx: %w", u, v, ErrInvalidInput)
			}

			y := new(big.Int).Div(nx, uBig)
			z := new(big.Int).Div(nx, vBig)

			sol := &Solution{X: x, Y: y, Z: z}
			if err := VerifySolution(n, sol); err != nil {
				return nil, fmt.Errorf("verification failed: %w", err)
			}
			return sol, nil
		},
	}
}

// SynthesizedAdditiveRules generates systematically proven families of the Additive Partition Lemma.
func SynthesizedAdditiveRules() []*Rule {
	return []*Rule{
		// A=7 = 1 + 6, x=(n+7)/4. Requires 6 | x <=> 24 | (n+7) <=> n = 17 mod 24
		AdditivePartitionFamily("RULE-ADDITIVE-7-1-6", "Additive Partition A=7 (1+6) Identity", 7, 1, 6, 24, 17),

		// A=7 = 3 + 4, x=(n+7)/4. Requires 12 | x <=> 48 | (n+7) <=> n = 41 mod 48
		AdditivePartitionFamily("RULE-ADDITIVE-7-3-4", "Additive Partition A=7 (3+4) Identity", 7, 3, 4, 48, 41),

		// A=11 = 1 + 10, x=(n+11)/4. Requires 10 | x <=> 40 | (n+11) <=> n = 29 mod 40
		AdditivePartitionFamily("RULE-ADDITIVE-11-1-10", "Additive Partition A=11 (1+10) Identity", 11, 1, 10, 40, 29),

		// A=11 = 3 + 8, x=(n+11)/4. Requires 24 | x <=> 96 | (n+11) <=> n = 85 mod 96
		AdditivePartitionFamily("RULE-ADDITIVE-11-3-8", "Additive Partition A=11 (3+8) Identity", 11, 3, 8, 96, 85),
	}
}
