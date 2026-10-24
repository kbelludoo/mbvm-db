package realeval

import (
	"fmt"
	"math/big"
)

// GenerateUnseenBenchmark20 constructs 20 brand-new, unexposed mathematical challenges
// specifically designed to require multi-lemma rule composition across the knowledge graph.
func GenerateUnseenBenchmark20() ([]EvalProblem, error) {
	problems := make([]EvalProblem, 0, 20)

	addConstruction := func(id string, nVal int64, desc string) {
		nBig := big.NewInt(nVal)
		problems = append(problems, EvalProblem{
			ID:     id,
			Type:   ProblemConstruction,
			N:      nBig,
			Prompt: desc,
		})
	}

	// 1. 8 Fresh Construction Problems on Certified Unexposed Primes (all 1 mod 840)
	// All require multi-lemma derivation: Mordell divisor formulation + mod 4 + mod 3 reduction
	addConstruction("U20-01", 21001, "Find positive integers x, y, z satisfying 4/21001 = 1/x + 1/y + 1/z (21001 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("U20-02", 26881, "Find positive integers x, y, z satisfying 4/26881 = 1/x + 1/y + 1/z (26881 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("U20-03", 29401, "Find positive integers x, y, z satisfying 4/29401 = 1/x + 1/y + 1/z (29401 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("U20-04", 35281, "Find positive integers x, y, z satisfying 4/35281 = 1/x + 1/y + 1/z (35281 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("U20-05", 41161, "Find positive integers x, y, z satisfying 4/41161 = 1/x + 1/y + 1/z (41161 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("U20-06", 52081, "Find positive integers x, y, z satisfying 4/52081 = 1/x + 1/y + 1/z (52081 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("U20-07", 63841, "Find positive integers x, y, z satisfying 4/63841 = 1/x + 1/y + 1/z (63841 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("U20-08", 89041, "Find positive integers x, y, z satisfying 4/89041 = 1/x + 1/y + 1/z (89041 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")

	// 2. 4 Multi-Lemma Algebraic Proof Problems
	problems = append(problems, EvalProblem{
		ID:     "U20-09",
		Type:   ProblemProof,
		Prompt: "Let n = 1 mod 12 and x = (n+3)/4. With a = 4x - n = 3 and t = nx, prove that for any prime p = 2 mod 3 with p | (t+p), (ay-t)(az-t) = t^2 yields integer solutions y and z.",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-10",
		Type:   ProblemProof,
		Prompt: "Prove that for any prime p >= 5 with p = 2 mod 3, the linear congruence 4k = -3 mod p always has a unique solution k mod p, and that n = 12m + 1 with m = k mod p satisfies both n = 1 mod 12 and n = -3 mod p.",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-11",
		Type:   ProblemProof,
		Prompt: "Prove that if n = 2 mod 3, the decomposition 4/n = 1/((n+1)/3) + 1/n + 1/(((n+1)/3)n) has all distinct positive integer denominators when n > 2.",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-12",
		Type:   ProblemProof,
		Prompt: "Prove that for any positive integer k, if q = 6k + 1, then 4q^2 - 3 = 1 mod 24.",
	})

	// 3. 4 Unexposed Refutation Problems
	problems = append(problems, EvalProblem{
		ID:     "U20-13",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For all odd integers q >= 1, n = 4q^2 - 3 is always prime.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-14",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For all primes n = 1 mod 24, x=(n+3)/4 always allows completing 4/n = 1/x + 1/y + 1/z with integers y, z.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-15",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'The Erdős-Straus equation 4/n = 1/x + 1/y + 1/z has a unique solution (x, y, z) with x <= y <= z for all primes n.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-16",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For n = 25, the choice x = (25+3)/4 = 7 allows completing 4/25 = 1/7 + 1/y + 1/z with integers y, z.'",
	})

	// 4. 4 Unexposed Modular Obstruction / Density Problems
	problems = append(problems, EvalProblem{
		ID:     "U20-17",
		Type:   ProblemObstruction,
		Prompt: "Prove that there is no integer q such that 4q^2 - 3 is congruent to 361 modulo 840, by analyzing quadratic residues modulo 7.",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-18",
		Type:   ProblemObstruction,
		Prompt: "Find the unique arithmetic progression modulo 19320 satisfying n = 529 mod 840 and n = -3 mod 23, and prove its uniqueness.",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-19",
		Type:   ProblemObstruction,
		Prompt: "Prove that no integer n can simultaneously satisfy n = 1 mod 24 and n = -3 mod 3.",
	})
	problems = append(problems, EvalProblem{
		ID:     "U20-20",
		Type:   ProblemObstruction,
		Prompt: "Prove that for any prime p = 2 mod 3 with p >= 5, the proportion of residue classes modulo 12p that satisfy both n = 1 mod 12 and n = -3 mod p is exactly 1/(12p).",
	})

	if err := validateUnseenDefinitions(problems); err != nil {
		return nil, fmt.Errorf("unseen benchmark validation failed: %w", err)
	}

	return problems, nil
}

func validateUnseenDefinitions(problems []EvalProblem) error {
	if len(problems) != 20 {
		return fmt.Errorf("unseen benchmark must contain exactly 20 problems, got %d", len(problems))
	}

	// 1. Rigorous primality certification of unexposed primes via Miller-Rabin
	unexposedPrimes := []int64{21001, 26881, 29401, 35281, 41161, 52081, 63841, 89041}
	for _, pr := range unexposedPrimes {
		bPr := big.NewInt(pr)
		if !bPr.ProbablyPrime(20) {
			return fmt.Errorf("declared unexposed prime %d failed Miller-Rabin primality test", pr)
		}
		if pr%840 != 1 {
			return fmt.Errorf("prime %d is not congruent to 1 mod 840: got %d", pr, pr%840)
		}
	}

	// 2. Certification of U20-13 refutation counterexample: q=9 => 4(81)-3 = 321 = 3 * 107 (composite)
	q13 := int64(9)
	n13 := 4*q13*q13 - 3
	if n13 != 321 || 321%3 != 0 {
		return fmt.Errorf("U20-13 counterexample q=9 failed: n=%d", n13)
	}

	// 3. Certification of U20-18 progression: 529 mod 840 and -3 mod 23 => 14809 mod 19320
	if 14809%840 != 529 || 14809%23 != (23-3)%23 {
		return fmt.Errorf("U20-18 progression 14809 fails: %d mod 840, %d mod 23", 14809%840, 14809%23)
	}

	return nil
}
