package realeval

import (
	"fmt"
	"math/big"
	"strings"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// RetrievalResult bundles retrieved rules with audit metadata.
type RetrievalResult struct {
	Rules       []*erdosstraus.Rule
	Path        string
	RuleIDs     []string
	TokenWeight int
}

// KnowledgeBase encapsulates standard and parameterized rules with CSR graph.
type KnowledgeBase struct {
	AllRules    []*erdosstraus.Rule
	CSRGraph    *erdosstraus.MathCSRGraph
	InvertedMap map[int64][]*erdosstraus.Rule // Modulus -> rules (conventional inverted index)
}

// NewKnowledgeBase initializes the rule repository and CSR dependency graph.
func NewKnowledgeBase() *KnowledgeBase {
	base := erdosstraus.StandardSeedRules()
	hardRules := erdosstraus.HardClasses840ParametricRules()
	all := append(base, hardRules...)

	csr := erdosstraus.NewMathCSRGraph(all)

	inverted := make(map[int64][]*erdosstraus.Rule)
	for _, r := range all {
		if r.Modulus > 0 {
			inverted[r.Modulus] = append(inverted[r.Modulus], r)
		}
	}

	return &KnowledgeBase{
		AllRules:    all,
		CSRGraph:    csr,
		InvertedMap: inverted,
	}
}

// RetrieveFlat returns all rules in the knowledge base without filtering.
func (kb *KnowledgeBase) RetrieveFlat() RetrievalResult {
	ids := make([]string, len(kb.AllRules))
	tokens := 0
	for i, r := range kb.AllRules {
		ids[i] = r.ID
		tokens += r.TokenWeight
	}
	return RetrievalResult{
		Rules:       kb.AllRules,
		Path:        "Flat-Dump:AllRules(Unfiltered)",
		RuleIDs:     ids,
		TokenWeight: tokens,
	}
}

// RetrieveFiltered uses a conventional inverted index without graph chaining.
func (kb *KnowledgeBase) RetrieveFiltered(n *big.Int) RetrievalResult {
	if n == nil {
		for _, r := range kb.AllRules {
			if r.ID == "RULE-MORDELL-DIVISOR" {
				return RetrievalResult{
					Rules:       []*erdosstraus.Rule{r},
					Path:        "InvertedIndex:Universal(RULE-MORDELL-DIVISOR)",
					RuleIDs:     []string{r.ID},
					TokenWeight: r.TokenWeight,
				}
			}
		}
	}

	var matched []*erdosstraus.Rule
	var ids []string
	tokens := 0

	candidateModuli := []int64{9240, 14280, 19320, 840, 24, 12, 8, 4, 3, 2}
	for _, m := range candidateModuli {
		if rules, ok := kb.InvertedMap[m]; ok {
			for _, r := range rules {
				if r.Covers(n) {
					matched = append(matched, r)
					ids = append(ids, r.ID)
					tokens += r.TokenWeight
				}
			}
			if len(matched) > 0 {
				break
			}
		}
	}

	if len(matched) == 0 {
		for _, r := range kb.AllRules {
			if r.ID == "RULE-MORDELL-DIVISOR" {
				matched = append(matched, r)
				ids = append(ids, r.ID)
				tokens += r.TokenWeight
				break
			}
		}
	}

	return RetrievalResult{
		Rules:       matched,
		Path:        fmt.Sprintf("ConventionalInvertedIndex:ModulusMatch(%s)", strings.Join(ids, ",")),
		RuleIDs:     ids,
		TokenWeight: tokens,
	}
}

// RetrieveMBVMCSR uses CSR adjacency traversal to extract active subgraph with semantic pruning.
func (kb *KnowledgeBase) RetrieveMBVMCSR(n *big.Int) RetrievalResult {
	if n == nil {
		idx, ok := kb.CSRGraph.IDToIdx["RULE-MORDELL-DIVISOR"]
		if !ok {
			return kb.RetrieveFiltered(n)
		}
		edgeCount := 0
		kb.CSRGraph.ForEachNeighbor(idx, func(int, erdosstraus.RelationType) bool {
			edgeCount++
			return true
		})
		rule := kb.AllRules[idx]
		path := fmt.Sprintf("CSR-Traversal:Root(%s)->Neighbors(%d)", rule.ID, edgeCount)
		return RetrievalResult{
			Rules:       []*erdosstraus.Rule{rule},
			Path:        path,
			RuleIDs:     []string{rule.ID},
			TokenWeight: rule.TokenWeight,
		}
	}

	// 1. Locate primary matching rule
	primaryIdx := -1
	candidateModuli := []int64{9240, 14280, 19320, 840, 24, 12, 8, 4, 3, 2}
	for _, m := range candidateModuli {
		if rules, ok := kb.InvertedMap[m]; ok {
			for _, r := range rules {
				if r.Covers(n) {
					primaryIdx = kb.CSRGraph.IDToIdx[r.ID]
					break
				}
			}
			if primaryIdx >= 0 {
				break
			}
		}
	}

	if primaryIdx < 0 {
		primaryIdx = kb.CSRGraph.IDToIdx["RULE-MORDELL-DIVISOR"]
	}

	primaryRule := kb.AllRules[primaryIdx]
	retrieved := []*erdosstraus.Rule{primaryRule}
	ids := []string{primaryRule.ID}
	tokens := primaryRule.TokenWeight

	// 2. CSR graph traversal with semantic prerequisite pruning:
	// Prune outgoing branches whose modular preconditions are contradicted by n.
	pathSteps := []string{fmt.Sprintf("Root(%s)", primaryRule.ID)}

	kb.CSRGraph.ForEachNeighbor(primaryIdx, func(target int, rel erdosstraus.RelationType) bool {
		depRule := kb.AllRules[target]

		// Semantic pruning must match the earlier allocating API exactly.
		if depRule.Modulus > 0 && !depRule.Covers(n) {
			return true
		}

		retrieved = append(retrieved, depRule)
		ids = append(ids, depRule.ID)
		tokens += depRule.TokenWeight
		pathSteps = append(pathSteps, fmt.Sprintf("Edge[Rel=%d]->%s", rel, depRule.ID))
		return true
	})

	return RetrievalResult{
		Rules:       retrieved,
		Path:        fmt.Sprintf("CSR-SmartSlice:%s", strings.Join(pathSteps, "->")),
		RuleIDs:     ids,
		TokenWeight: tokens,
	}
}
