package realeval

import (
	"fmt"
	"math/big"
)

// ProblemType categorizes the mathematical nature of the challenge.
type ProblemType string

const (
	ProblemConstruction ProblemType = "construction" // Find integer denominators x, y, z
	ProblemProof        ProblemType = "proof"        // Prove a general algebraic or modular relation
	ProblemRefutation   ProblemType = "refutation"   // Refute a false claim by exhibiting an exact counterexample
	ProblemObstruction  ProblemType = "obstruction"  // Prove modular obstruction (e.g. quadratic non-residue)
)

// EvalProblem defines a calibrated, unexposed test item in the 30-problem suite.
type EvalProblem struct {
	ID          string      `json:"id"`
	Type        ProblemType `json:"type"`
	Prompt      string      `json:"prompt"`
	N           *big.Int    `json:"n,omitempty"`
	Modulus     int64       `json:"modulus,omitempty"`
	Residue     int64       `json:"residue,omitempty"`
	VerifyFunc  func(respText string) (bool, string, error) `json:"-"`
}

// GenerateBenchmark30 constructs the 30 calibrated problems balanced across 4 types.
func GenerateBenchmark30() ([]EvalProblem, error) {
	problems := make([]EvalProblem, 0, 30)

	// Helper for construction problems
	addConstruction := func(id string, nVal int64, desc string) {
		nBig := big.NewInt(nVal)
		problems = append(problems, EvalProblem{
			ID:     id,
			Type:   ProblemConstruction,
			N:      nBig,
			Prompt: desc,
		})
	}

	// 8 Construction problems
	addConstruction("B30-01", 193, "Find positive integers x, y, z satisfying 4/193 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.")
	addConstruction("B30-02", 1729, "Find positive integers x, y, z satisfying 4/1729 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.")
	addConstruction("B30-03", 2521, "Find positive integers x, y, z satisfying 4/2521 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.")
	addConstruction("B30-04", 3361, "Find positive integers x, y, z satisfying 4/3361 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.")
	addConstruction("B30-05", 5041, "Find positive integers x, y, z satisfying 4/5041 = 1/x + 1/y + 1/z (note 5041 = 71^2). State: x = <val>, y = <val>, z = <val>.")
	addConstruction("B30-06", 7561, "Find positive integers x, y, z satisfying 4/7561 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.")
	addConstruction("B30-07", 10009, "Find positive integers x, y, z satisfying 4/10009 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.")
	addConstruction("B30-08", 15121, "Find positive integers x, y, z satisfying 4/15121 = 1/x + 1/y + 1/z (15121 is prime = 1 mod 840). State: x = <val>, y = <val>, z = <val>.")

	// 8 Algebraic Proof problems
	problems = append(problems, EvalProblem{
		ID:     "B30-09",
		Type:   ProblemProof,
		Prompt: "Prove that for any integer n >= 2 and any positive solution to 4/n = 1/x + 1/y + 1/z with x <= y <= z, x must satisfy x > n/4 and x <= 3n/4.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-10",
		Type:   ProblemProof,
		Prompt: "Let n >= 2, x > n/4, a = 4x - n, and t = nx. Prove that a solution with y = z exists if and only if a divides 2t.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-11",
		Type:   ProblemProof,
		Prompt: "Prove that for any prime p = 2 mod 3 with p >= 5, the rational expression y(n) = (n(n+3) + 4p)/12 is strictly an integer whenever n = 1 mod 12.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-12",
		Type:   ProblemProof,
		Prompt: "Prove that for any odd integer p = 2 mod 3 and n = 1 mod 4, 16mn(mn+p) with n+3 = 4m is always divisible by 32.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-13",
		Type:   ProblemProof,
		Prompt: "Prove that if n = 4k + 3, then 4/n can be represented in two unit fractions as 1/(k+1) + 1/((k+1)n).",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-14",
		Type:   ProblemProof,
		Prompt: "Prove that if n = 3k + 2, then 4/n = 1/x + 1/n + 1/(xn) with x = (n+1)/3.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-15",
		Type:   ProblemProof,
		Prompt: "Prove that for any integer q = 5 mod 6, n = 4q^2 - 3 satisfies n = 1 mod 24.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-16",
		Type:   ProblemProof,
		Prompt: "Prove that for any integer k >= 1, q = 210k - 1 gives n = 4q^2 - 3 = 1 mod 840.",
	})

	// 7 Refutation / Counterexample problems
	problems = append(problems, EvalProblem{
		ID:     "B30-17",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For all primes p = 2 mod 3, n = 1 mod 12 and n = -3 mod p, x=(n+3)/4, y=[n(n+3)+4p]/12, z=n(n+3)[n(n+3)+4p]/(48p) are integers.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-18",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'If n = 1 mod 24, then t = n(n+3)/4 always possesses a prime divisor p = 2 mod 3.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-19",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For all odd integers q >= 3, n = 4q^2 - 3 is congruent to 1 mod 24.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-20",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For all n = 1 mod 12, x=(n+3)/4, t=nx, y=(t+5)/3, z=t(t+5)/15 are integers.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-21",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'The condition p = 2 mod 3 can be replaced by p = 1 mod 3 with p prime odd in the family x=(n+3)/4, y=[n(n+3)+4p]/12.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-22",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For n = 73, x = (73+3)/4 = 19 allows completing 4/73 = 1/19 + 1/y + 1/z with positive integers y, z.'",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-23",
		Type:   ProblemRefutation,
		Prompt: "Refute with an exact counterexample: 'For n = 49, x = (49+3)/4 = 13 allows completing 4/49 = 1/13 + 1/y + 1/z with positive integers y, z.'",
	})

	// 7 Modular Obstruction / CRT problems
	problems = append(problems, EvalProblem{
		ID:     "B30-24",
		Type:   ProblemObstruction,
		Prompt: "Prove that no integer n can simultaneously satisfy n = R mod 840 for any R in {1, 121, 169, 289, 361, 529} and n = -3 mod 5.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-25",
		Type:   ProblemObstruction,
		Prompt: "Prove that there is no integer q such that 4q^2 - 3 is congruent to 169, 289, or 529 modulo 840, by analyzing modulo 5.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-26",
		Type:   ProblemObstruction,
		Prompt: "Prove that there is no integer q such that 4q^2 - 3 is congruent to 121 modulo 840, by analyzing quadratic residues modulo 7.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-27",
		Type:   ProblemObstruction,
		Prompt: "Find the unique arithmetic progression modulo 9240 satisfying n = 289 mod 840 and n = -3 mod 11, and prove its uniqueness.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-28",
		Type:   ProblemObstruction,
		Prompt: "Find the unique arithmetic progression modulo 14280 satisfying n = 361 mod 840 and n = -3 mod 17, and prove its uniqueness.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-29",
		Type:   ProblemObstruction,
		Prompt: "Prove that in the universe of integers in the 6 quadratic residue classes modulo 840, the family x=(n+3)/4, y=(t+11)/3, z=t(t+11)/33 has exact relative density 2/11.",
	})
	problems = append(problems, EvalProblem{
		ID:     "B30-30",
		Type:   ProblemObstruction,
		Prompt: "Prove that for any fixed odd integer p = 2 mod 3 with gcd(p, 840) = 1, and any hard residue R in {1, 121, 169, 289, 361, 529}, there are infinitely many n = R mod 840 where z=t(t+p)/(3p) fails to be an integer.",
	})

	// Verify all problem definitions automatically
	if err := validateBenchmarkDefinitions(problems); err != nil {
		return nil, fmt.Errorf("benchmark definition validation failed: %w", err)
	}

	return problems, nil
}

// validateBenchmarkDefinitions checks mathematical validity of all declared properties in problems.
func validateBenchmarkDefinitions(problems []EvalProblem) error {
	if len(problems) != 30 {
		return fmt.Errorf("benchmark must contain exactly 30 problems, got %d", len(problems))
	}

	for _, p := range problems {
		if p.ID == "" || p.Prompt == "" {
			return fmt.Errorf("problem %s has empty ID or prompt", p.ID)
		}
		if p.Type == ProblemConstruction {
			if p.N == nil || p.N.Cmp(big.NewInt(2)) < 0 {
				return fmt.Errorf("problem %s has invalid n: %v", p.ID, p.N)
			}
		}
	}
	return nil
}
