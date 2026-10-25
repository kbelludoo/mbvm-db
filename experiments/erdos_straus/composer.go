package erdosstraus

import (
	"fmt"
	"math/big"
	"slices"
)

// ComposedCandidate encapsulates a proposed new rule created by composing existing rules.
type ComposedCandidate struct {
	DerivedRule   *Rule
	ParentIDs     []string
	CompositionOp string
	Verified      bool
	Refutation    string
}

// RuleComposer manages rule composition, CRT fusion, and hypothesis validation.
type RuleComposer struct {
	BaseRules []*Rule
	Graph     *MathCSRGraph
}

// NewRuleComposer creates a composer with initialized knowledge base and CSR graph.
func NewRuleComposer(baseRules []*Rule) *RuleComposer {
	graph := NewMathCSRGraph(baseRules)
	return &RuleComposer{
		BaseRules: baseRules,
		Graph:     graph,
	}
}

// ComposeCRT combines two modular rules using the Chinese Remainder Theorem.
func (c *RuleComposer) ComposeCRT(r1, r2 *Rule) (*ComposedCandidate, error) {
	if r1.Modulus <= 0 || r2.Modulus <= 0 {
		return nil, fmt.Errorf("both rules must have positive moduli: %w", ErrInvalidInput)
	}

	gcd := new(big.Int).GCD(nil, nil, big.NewInt(r1.Modulus), big.NewInt(r2.Modulus))
	if gcd.Int64() != 1 {
		return nil, fmt.Errorf("moduli %d and %d are not coprime (gcd=%d): %w",
			r1.Modulus, r2.Modulus, gcd.Int64(), ErrContradictorySystem)
	}

	composedMod := r1.Modulus * r2.Modulus
	var combinedResidues []int64

	// Solve CRT for each residue pair: n = a mod m1, n = b mod m2
	// n = a + m1 * [ (b - a)*inv(m1, m2) mod m2 ]
	m1Big := big.NewInt(r1.Modulus)
	m2Big := big.NewInt(r2.Modulus)

	invM1 := new(big.Int).ModInverse(m1Big, m2Big)
	if invM1 == nil {
		return nil, fmt.Errorf("modular inverse does not exist: %w", ErrContradictorySystem)
	}

	for _, a := range r1.Residues {
		for _, b := range r2.Residues {
			diff := big.NewInt(b - a)
			diff.Mul(diff, invM1)
			diff.Mod(diff, m2Big)
			res := a + r1.Modulus*diff.Int64()
			res = (res%composedMod + composedMod) % composedMod
			if !slices.Contains(combinedResidues, res) {
				combinedResidues = append(combinedResidues, res)
			}
		}
	}

	slices.Sort(combinedResidues)

	derivedID := fmt.Sprintf("COMPOSED-CRT-%s-x-%s", r1.ID, r2.ID)
	if _, exists := c.Graph.IDToIdx[derivedID]; exists {
		return nil, fmt.Errorf("CRT rule %s is already registered: %w", derivedID, ErrInvalidInput)
	}
	derivedRule := &Rule{
		ID:           derivedID,
		Name:         fmt.Sprintf("CRT Composition of %s and %s", r1.Name, r2.Name),
		Hypotheses:   append(slices.Clone(r1.Hypotheses), r2.Hypotheses...),
		Conclusion:   fmt.Sprintf("Combined residue coverage mod %d", composedMod),
		Modulus:      composedMod,
		Residues:     combinedResidues,
		Dependencies: []string{r1.ID, r2.ID},
		BibSource:    "Constructive CRT composition via MBVM-DB",
		Status:       StatusCandidate,
		TokenWeight:  r1.TokenWeight + r2.TokenWeight,
		SolveFunc: func(n *big.Int) (*Solution, error) {
			// Try first rule, fallback to second
			if sol, err := r1.Apply(n); err == nil {
				return sol, nil
			}
			return r2.Apply(n)
		},
	}

	candidate := &ComposedCandidate{
		DerivedRule:   derivedRule,
		ParentIDs:     []string{r1.ID, r2.ID},
		CompositionOp: "CRT",
	}

	// Verify hypotheses and validate candidate on concrete instances
	if err := c.ValidateCandidate(candidate, 20); err != nil {
		candidate.Refutation = err.Error()
		candidate.DerivedRule.Status = StatusRefuted
	} else {
		// Register the derived rule BEFORE adding composition edges.
		// A graph built solely from BaseRules cannot connect to an unknown target ID;
		// ignoring AddEdge errors previously left "verified" rules disconnected.
		//
		// Rebuild locally first, then publish the complete graph atomically.
		// Rule.Dependencies supplies the derived -> parent dependency edges.
		nextRules := append(slices.Clone(c.BaseRules), derivedRule)
		nextGraph := NewMathCSRGraph(nextRules)
		// NewMathCSRGraph rebuilds dependency/generalization edges from the
		// catalog, but previously inserted RelComposition edges are dynamic.
		// Preserve all earlier composition edges before attaching the new rule.
		batch := make([]CSRNamedEdge, 0, 2*len(candidate.ParentIDs))
		for sourceIdx, sourceID := range c.Graph.RuleIDs {
			invalidTarget := -1
			c.Graph.ForEachNeighbor(sourceIdx, func(target int, rel RelationType) bool {
				if rel != RelComposition {
					return true
				}
				if target < 0 || target >= len(c.Graph.RuleIDs) {
					invalidTarget = target
					return false
				}
				batch = append(batch, CSRNamedEdge{
					SourceID: sourceID, TargetID: c.Graph.RuleIDs[target], Rel: RelComposition,
				})
				return true
			})
			if invalidTarget >= 0 {
				return nil, fmt.Errorf("invalid existing composition target %d for %s: %w",
					invalidTarget, sourceID, ErrInvalidInput)
			}
		}
		for _, parentID := range candidate.ParentIDs {
			batch = append(batch, CSRNamedEdge{
				SourceID: parentID, TargetID: derivedRule.ID, Rel: RelComposition,
			})
		}
		// Add all old and new composition edges in one atomic CSR repack,
		// rather than triggering a full rebuild for every single edge.
		if err := nextGraph.AddEdges(batch); err != nil {
			return nil, fmt.Errorf("register CRT composition batch for %s: %w",
				derivedRule.ID, err)
		}
		c.BaseRules = nextRules
		*c.Graph = *nextGraph
		candidate.Verified = true // finite-instance check, not a universal formal proof
		candidate.DerivedRule.Status = StatusVerified
	}

	return candidate, nil
}

// DeriveMordellFamily instantiates a Mordell parametric family for parameters (a, b).
func (c *RuleComposer) DeriveMordellFamily(a, b int64) (*ComposedCandidate, error) {
	if a <= 0 || b <= 0 {
		return nil, fmt.Errorf("mordell parameters a, b must be > 0: %w", ErrInvalidInput)
	}
	modulus := 4*a*b - 1
	targetResidue := (modulus - (a % modulus)) % modulus

	ruleID := fmt.Sprintf("RULE-MORDELL-A%d-B%d", a, b)
	rule := &Rule{
		ID:         ruleID,
		Name:       fmt.Sprintf("Mordell Sieve Identity (a=%d, b=%d)", a, b),
		Hypotheses: []string{fmt.Sprintf("n = -%d mod %d (i.e. %d divides n + %d)", a, modulus, modulus, a)},
		Conclusion: fmt.Sprintf("Solvable via divisor parameterization with modulus %d", modulus),
		Modulus:    modulus,
		Residues:   []int64{targetResidue},
		BibSource:  "Mordell (1967) Diophantine parameterization",
		Status:     StatusCandidate,
		SolveFunc: func(n *big.Int) (*Solution, error) {
			return solveViaDivisorSearch(n, 100)
		},
	}

	candidate := &ComposedCandidate{
		DerivedRule:   rule,
		ParentIDs:     []string{"RULE-MORDELL-DIVISOR"},
		CompositionOp: "MordellParametricInstantiate",
	}

	if err := c.ValidateCandidate(candidate, 10); err != nil {
		candidate.Refutation = err.Error()
		candidate.DerivedRule.Status = StatusRefuted
	} else {
		candidate.Verified = true
		candidate.DerivedRule.Status = StatusVerified
	}

	return candidate, nil
}

// ValidateCandidate checks that the proposed candidate produces valid Erdős-Straus solutions.
func (c *RuleComposer) ValidateCandidate(cand *ComposedCandidate, sampleCount int) error {
	rule := cand.DerivedRule
	if rule.Modulus <= 0 || len(rule.Residues) == 0 {
		return nil
	}

	mod := big.NewInt(rule.Modulus)
	tested := 0
	for k := int64(0); tested < sampleCount && k < 1000; k++ {
		for _, res := range rule.Residues {
			n := new(big.Int).Mul(big.NewInt(k), mod)
			n.Add(n, big.NewInt(res))
			if n.Cmp(big.NewInt(2)) < 0 {
				continue
			}

			sol, err := rule.Apply(n)
			if err != nil {
				return fmt.Errorf("candidate solver failed for sample n=%s: %w", n.String(), err)
			}
			if err := VerifySolution(n, sol); err != nil {
				return fmt.Errorf("candidate produced invalid solution for n=%s: %w", n.String(), err)
			}
			tested++
			if tested >= sampleCount {
				break
			}
		}
	}

	return nil
}

// ErrContradictorySystem is returned when modular constraints have no common solution.
var ErrContradictorySystem = fmt.Errorf("contradictory modular system")
