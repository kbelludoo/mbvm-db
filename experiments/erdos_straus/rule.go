package erdosstraus

import (
	"fmt"
	"math/big"
)

// RuleStatus represents the formal state of an algebraic rule or lemma.
type RuleStatus string

const (
	StatusVerified  RuleStatus = "verified"
	StatusCandidate RuleStatus = "candidate"
	StatusRefuted   RuleStatus = "refuted"
)

// Solution holds positive integers x, y, z satisfying 4/n = 1/x + 1/y + 1/z.
type Solution struct {
	X *big.Int
	Y *big.Int
	Z *big.Int
}

// Rule defines a symbolic mathematical lemma, parametric identity, or modular sieve.
type Rule struct {
	ID           string
	Name         string
	Hypotheses   []string
	Conclusion   string
	Modulus      int64
	Residues     []int64
	Dependencies []string
	BibSource    string
	ProofText    string
	Status       RuleStatus
	SolveFunc    func(n *big.Int) (*Solution, error)
	TokenWeight  int
}

// Covers returns true if n mod Modulus matches any covered residue class.
func (r *Rule) Covers(n *big.Int) bool {
	if r.Modulus <= 0 || len(r.Residues) == 0 {
		return false
	}
	mod := big.NewInt(r.Modulus)
	rem := new(big.Int).Mod(n, mod).Int64()
	for _, res := range r.Residues {
		if rem == res {
			return true
		}
	}
	return false
}

// Apply attempts to compute positive integers (x, y, z) for a given n.
func (r *Rule) Apply(n *big.Int) (*Solution, error) {
	if r.SolveFunc == nil {
		return nil, fmt.Errorf("rule %s has no evaluation solver: %w", r.ID, ErrNoSolver)
	}
	sol, err := r.SolveFunc(n)
	if err != nil {
		return nil, fmt.Errorf("rule %s evaluation failed for n=%s: %w", r.ID, n.String(), err)
	}
	return sol, nil
}

// ErrNoSolver is returned when a rule is declarative without an executable solver.
var ErrNoSolver = fmt.Errorf("no solver attached to rule")
