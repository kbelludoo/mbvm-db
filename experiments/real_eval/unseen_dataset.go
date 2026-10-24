package realeval

import "math/big"

// UnseenEvaluationDataset returns 10 fresh, unexposed problems for live LLM benchmarking.
func UnseenEvaluationDataset() []MathChallenge {
	return []MathChallenge{
		{
			ID:          "UNSEEN-01",
			Description: "Decompose 4/n into three unit fractions for prime n = 193 (where 193 = 1 mod 24 and 193 = 6 mod 11).",
			N:           big.NewInt(193),
			Modulus:     24,
			Residue:     1,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-02",
			Description: "Decompose 4/n for the Ramanujan-Hardy taxicab prime n = 1729? (1729 is composite 7*13*19, 1729 = 1 mod 24, 1729 = 49 mod 840).",
			N:           big.NewInt(1729),
			Modulus:     840,
			Residue:     49,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-03",
			Description: "Decompose 4/n for prime n = 5041? 5041 = 71^2 (quadratic square, 5041 = 1 mod 840).",
			N:           big.NewInt(5041),
			Modulus:     840,
			Residue:     1,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-04",
			Description: "Decompose 4/n for prime n = 3361 (where 3361 = 1 mod 840).",
			N:           big.NewInt(3361),
			Modulus:     840,
			Residue:     1,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-05",
			Description: "Decompose 4/n for prime n = 7561 (where 7561 = 1 mod 840 and 7561 = 20 mod 23).",
			N:           big.NewInt(7561),
			Modulus:     840,
			Residue:     1,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-06",
			Description: "Decompose 4/n for prime n = 9241 (where 9241 = 1 mod 9240).",
			N:           big.NewInt(9241),
			Modulus:     9240,
			Residue:     1,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-07",
			Description: "Decompose 4/n for prime n = 10009 (where 10009 = 769 mod 840).",
			N:           big.NewInt(10009),
			Modulus:     840,
			Residue:     769,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-08",
			Description: "Decompose 4/n for prime n = 16801 (where 16801 = 1 mod 840).",
			N:           big.NewInt(16801),
			Modulus:     840,
			Residue:     1,
			TargetType:  "solve_fractions",
		},
		{
			ID:          "UNSEEN-09",
			Description: "Refute or solve: Can 4/n be written as 1/x + 1/y + 1/z with y=z for prime n = 193?",
			N:           big.NewInt(193),
			Modulus:     24,
			Residue:     1,
			TargetType:  "refute_claim",
		},
		{
			ID:          "UNSEEN-10",
			Description: "Decompose 4/n for cryptographic-scale prime n = 100000007 (where 100000007 = 23 mod 24).",
			N:           big.NewInt(100000007),
			Modulus:     24,
			Residue:     23,
			TargetType:  "solve_fractions",
		},
	}
}
