package realeval

import (
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"
)

// The legacy reference is intentionally based on Neighbors(), the API used
// before the zero-copy migration. It checks byte-for-byte prompt metadata.
func TestCSRForEachRetrievalPreservesLegacyPathAndRules(t *testing.T) {
	kb := NewKnowledgeBase()
	cases := []*big.Int{
		nil, big.NewInt(3), big.NewInt(5), big.NewInt(11),
		big.NewInt(49), big.NewInt(26881), big.NewInt(99991),
	}
	for _, n := range cases {
		got := kb.RetrieveMBVMCSR(n)
		if len(got.Rules) == 0 {
			t.Fatalf("empty retrieval for n=%v", n)
		}
		idx, exists := kb.CSRGraph.IDToIdx[got.Rules[0].ID]
		if !exists {
			t.Fatalf("missing root in CSR index: %s", got.Rules[0].ID)
		}
		edges := kb.CSRGraph.Neighbors(idx)
		if n == nil {
			want := fmt.Sprintf("CSR-Traversal:Root(%s)->Neighbors(%d)", got.Rules[0].ID, len(edges))
			if got.Path != want {
				t.Fatalf("nil-query path: got %q, want %q", got.Path, want)
			}
			continue
		}
		wantIDs := []string{got.Rules[0].ID}
		path := []string{fmt.Sprintf("Root(%s)", got.Rules[0].ID)}
		wantTokens := got.Rules[0].TokenWeight
		for _, edge := range edges {
			dep := kb.AllRules[edge.Target]
			if dep.Modulus > 0 && !dep.Covers(n) {
				continue
			}
			wantIDs = append(wantIDs, dep.ID)
			wantTokens += dep.TokenWeight
			path = append(path, fmt.Sprintf("Edge[Rel=%d]->%s", edge.Rel, dep.ID))
		}
		wantPath := "CSR-SmartSlice:" + strings.Join(path, "->")
		if !slices.Equal(got.RuleIDs, wantIDs) || got.TokenWeight != wantTokens || got.Path != wantPath {
			t.Fatalf("n=%s: new iteration altered CSR retrieval: ids=%v expected=%v tokens=%d expected=%d path=%q expected=%q",
				n.String(), got.RuleIDs, wantIDs, got.TokenWeight, wantTokens, got.Path, wantPath)
		}
	}
}
