package erdosstraus

import (
	"math/big"
	"time"
)

// GroupType labels the experimental condition.
type GroupType string

const (
	GroupA GroupType = "Group A (Direct Baseline)"
	GroupB GroupType = "Group B (Conventional Retrieval)"
	GroupC GroupType = "Group C (MBVM-DB Full: CSR + HDC + Composer)"
	GroupD GroupType = "Group D (MBVM-DB No-HDC: CSR + Map + Composer)"
)

// BenchmarkItem represents a single challenge n to be solved and verified.
type BenchmarkItem struct {
	N           *big.Int
	Category    string // "even", "mod4_3", "mod3_2", "mod8_5", "hard_840"
	Description string
}

// GroupResult stores empirical metrics for a single group on the test benchmark.
type GroupResult struct {
	Group           GroupType     `json:"group"`
	SolvedCount     int           `json:"solved_count"`
	TotalProblems   int           `json:"total_problems"`
	SuccessRate     float64       `json:"success_rate"`
	TotalTokens     int           `json:"total_tokens"`
	AverageTokens   float64       `json:"average_tokens"`
	ExecutionTime   time.Duration `json:"execution_time"`
	TotalAttempts   int           `json:"total_attempts"`
	RulesRetrieved  int           `json:"rules_retrieved"`
	InvalidReturned int           `json:"invalid_returned"`
	MemoryType      string        `json:"memory_type"`
}

// BenchmarkSuite holds test items across various difficulty tiers.
func BenchmarkSuite() []BenchmarkItem {
	return []BenchmarkItem{
		// Tier 1: Elementary / Standard Moduli
		{N: big.NewInt(100), Category: "even", Description: "n=100 (composite even)"},
		{N: big.NewInt(103), Category: "mod4_3", Description: "n=103 (prime, 103 = 3 mod 4)"},
		{N: big.NewInt(107), Category: "mod3_2", Description: "n=107 (prime, 107 = 2 mod 3)"},
		{N: big.NewInt(109), Category: "mod8_5", Description: "n=109 (prime, 109 = 5 mod 8)"},

		// Tier 2: Tricky Primes (n = 1 mod 24, not mod 8 = 5)
		{N: big.NewInt(73), Category: "mod24_1", Description: "n=73 (prime, 73 = 1 mod 24)"},
		{N: big.NewInt(97), Category: "mod24_1", Description: "n=97 (prime, 97 = 1 mod 24)"},

		// Tier 3: Hard Respective Quadratic Residues Mod 840 (the open challenge subset)
		// 840 hard residues: {1, 121, 169, 289, 361, 529} mod 840
		{N: big.NewInt(1009), Category: "hard_840", Description: "n=1009 (prime, 1009 = 169 mod 840)"},
		{N: big.NewInt(1201), Category: "hard_840", Description: "n=1201 (prime, 1201 = 361 mod 840)"},
		{N: big.NewInt(1609), Category: "hard_840", Description: "n=1609 (prime, 1609 = 769 mod 840)"},
		{N: big.NewInt(2521), Category: "hard_840", Description: "n=2521 (prime, 2521 = 1 mod 840)"},

		// Tier 4: Large Primes
		{N: big.NewInt(1000000007), Category: "large_prime", Description: "n=1000000007 (cryptographic scale)"},
	}
}

// RunControlledExperiment executes the 4-group scientific trial under strict controls.
func RunControlledExperiment() ([]GroupResult, error) {
	suite := BenchmarkSuite()
	results := make([]GroupResult, 0, 4)

	// Group A: Direct baseline (pure unguided search, no memory/rules)
	resA := runGroupA(suite)
	results = append(results, resA)

	// Group B: Conventional retrieval (flat rule library scanned on every query)
	resB := runGroupB(suite)
	results = append(results, resB)

	// Group C: MBVM-DB Full (CSR graph + HDC memory + rule composer)
	resC := runGroupC(suite)
	results = append(results, resC)

	// Group D: MBVM-DB No-HDC (CSR graph + exact index + rule composer)
	resD := runGroupD(suite)
	results = append(results, resD)

	return results, nil
}

func runGroupA(suite []BenchmarkItem) GroupResult {
	start := time.Now()
	solved := 0
	tokens := 0
	attempts := 0
	invalid := 0

	// Group A has no rule library. It tries a naive search x in [n/4+1, n/4+20].
	for _, item := range suite {
		attempts++
		tokens += 120 // baseline prompt and search overhead
		sol, err := solveViaDivisorSearch(item.N, 20)
		if err == nil && sol != nil {
			if vErr := VerifySolution(item.N, sol); vErr == nil {
				solved++
			} else {
				invalid++
			}
		}
	}

	return GroupResult{
		Group:           GroupA,
		SolvedCount:     solved,
		TotalProblems:   len(suite),
		SuccessRate:     float64(solved) / float64(len(suite)),
		TotalTokens:     tokens,
		AverageTokens:   float64(tokens) / float64(len(suite)),
		ExecutionTime:   time.Since(start),
		TotalAttempts:   attempts,
		RulesRetrieved:  0,
		InvalidReturned: invalid,
		MemoryType:      "none (direct)",
	}
}

func runGroupB(suite []BenchmarkItem) GroupResult {
	start := time.Now()
	rules := StandardSeedRules()
	solved := 0
	tokens := 0
	attempts := 0
	retrieved := 0
	invalid := 0

	for _, item := range suite {
		attempts++
		// Group B dumps all rules into context (flat linear search)
		for _, r := range rules {
			tokens += r.TokenWeight
		}
		tokens += 80 // context query wrapper

		var candidateSol *Solution
		for _, r := range rules {
			retrieved++
			if r.Covers(item.N) {
				if sol, err := r.Apply(item.N); err == nil {
					candidateSol = sol
					break
				}
			}
		}
		if candidateSol == nil {
			// fallback to general divisor rule if present
			for _, r := range rules {
				if r.ID == "RULE-MORDELL-DIVISOR" {
					sol, err := r.Apply(item.N)
					if err == nil {
						candidateSol = sol
					}
					break
				}
			}
		}

		if candidateSol != nil {
			if vErr := VerifySolution(item.N, candidateSol); vErr == nil {
				solved++
			} else {
				invalid++
			}
		}
	}

	return GroupResult{
		Group:           GroupB,
		SolvedCount:     solved,
		TotalProblems:   len(suite),
		SuccessRate:     float64(solved) / float64(len(suite)),
		TotalTokens:     tokens,
		AverageTokens:   float64(tokens) / float64(len(suite)),
		ExecutionTime:   time.Since(start),
		TotalAttempts:   attempts,
		RulesRetrieved:  retrieved,
		InvalidReturned: invalid,
		MemoryType:      "flat searchable list",
	}
}

func runGroupC(suite []BenchmarkItem) GroupResult {
	start := time.Now()
	rules := StandardSeedRules()
	composer := NewRuleComposer(rules)
	hdc := NewHDCMemory(2026)

	// Index rules into HDC
	for _, r := range rules {
		hdc.EncodeRule(r)
	}

	// Expand knowledge base via composition
	_, _ = composer.ComposeCRT(rules[1], rules[2])      // Mod4-3 x Mod3-2
	mord7, _ := composer.DeriveMordellFamily(1, 2)     // 4(1)(2)-1 = 7
	mord11, _ := composer.DeriveMordellFamily(1, 3)    // 4(1)(3)-1 = 11
	if mord7 != nil && mord7.Verified {
		hdc.EncodeRule(mord7.DerivedRule)
	}
	if mord11 != nil && mord11.Verified {
		hdc.EncodeRule(mord11.DerivedRule)
	}

	candidateModuli := []int64{2, 3, 4, 7, 8, 11, 12, 24}
	solved := 0
	tokens := 0
	attempts := 0
	retrieved := 0
	invalid := 0

	for _, item := range suite {
		attempts++
		// Sliced retrieval via HDC: only retrieves top matching rule cluster
		bestRuleID, _ := hdc.QueryBestRule(item.N.Int64(), candidateModuli)
		retrieved++

		var activeRule *Rule
		for _, r := range composer.BaseRules {
			if r.ID == bestRuleID {
				activeRule = r
				break
			}
		}

		// Sliced token cost is only the specific retrieved rule + query (Smart Slicing)
		if activeRule != nil {
			tokens += activeRule.TokenWeight + 30
		} else {
			tokens += 60
		}

		var candidateSol *Solution
		if activeRule != nil && activeRule.Covers(item.N) {
			candidateSol, _ = activeRule.Apply(item.N)
		}

		// If top rule does not cover, query CSR neighbors or fallback to Mordell divisor
		if candidateSol == nil {
			for _, r := range composer.BaseRules {
				if r.Covers(item.N) {
					retrieved++
					tokens += r.TokenWeight
					candidateSol, _ = r.Apply(item.N)
					if candidateSol != nil {
						break
					}
				}
			}
		}

		if candidateSol == nil {
			// Universal solver
			for _, r := range composer.BaseRules {
				if r.ID == "RULE-MORDELL-DIVISOR" {
					retrieved++
					tokens += r.TokenWeight
					candidateSol, _ = r.Apply(item.N)
					break
				}
			}
		}

		if candidateSol != nil {
			if vErr := VerifySolution(item.N, candidateSol); vErr == nil {
				solved++
			} else {
				invalid++
			}
		}
	}

	return GroupResult{
		Group:           GroupC,
		SolvedCount:     solved,
		TotalProblems:   len(suite),
		SuccessRate:     float64(solved) / float64(len(suite)),
		TotalTokens:     tokens,
		AverageTokens:   float64(tokens) / float64(len(suite)),
		ExecutionTime:   time.Since(start),
		TotalAttempts:   attempts,
		RulesRetrieved:  retrieved,
		InvalidReturned: invalid,
		MemoryType:      "MBVM-DB (HDC + CSR + Composer)",
	}
}

func runGroupD(suite []BenchmarkItem) GroupResult {
	start := time.Now()
	rules := StandardSeedRules()
	composer := NewRuleComposer(rules)

	// Group D uses exact modulus-residue map instead of HDC
	type modResPair struct{ mod, res int64 }
	exactMap := make(map[modResPair]*Rule)
	for _, r := range rules {
		if r.Modulus > 0 {
			for _, res := range r.Residues {
				exactMap[modResPair{mod: r.Modulus, res: res}] = r
			}
		}
	}

	solved := 0
	tokens := 0
	attempts := 0
	retrieved := 0
	invalid := 0

	candidateModuli := []int64{2, 3, 4, 8, 12, 24}
	for _, item := range suite {
		attempts++
		var candidateSol *Solution
		for _, mod := range candidateModuli {
			rem := new(big.Int).Mod(item.N, big.NewInt(mod)).Int64()
			if r, ok := exactMap[modResPair{mod: mod, res: rem}]; ok {
				retrieved++
				tokens += r.TokenWeight + 25
				if sol, err := r.Apply(item.N); err == nil {
					candidateSol = sol
					break
				}
			}
		}

		if candidateSol == nil {
			for _, r := range composer.BaseRules {
				if r.ID == "RULE-MORDELL-DIVISOR" {
					retrieved++
					tokens += r.TokenWeight
					candidateSol, _ = r.Apply(item.N)
					break
				}
			}
		}

		if candidateSol != nil {
			if vErr := VerifySolution(item.N, candidateSol); vErr == nil {
				solved++
			} else {
				invalid++
			}
		}
	}

	return GroupResult{
		Group:           GroupD,
		SolvedCount:     solved,
		TotalProblems:   len(suite),
		SuccessRate:     float64(solved) / float64(len(suite)),
		TotalTokens:     tokens,
		AverageTokens:   float64(tokens) / float64(len(suite)),
		ExecutionTime:   time.Since(start),
		TotalAttempts:   attempts,
		RulesRetrieved:  retrieved,
		InvalidReturned: invalid,
		MemoryType:      "MBVM-DB No-HDC (Exact Map + CSR + Composer)",
	}
}
