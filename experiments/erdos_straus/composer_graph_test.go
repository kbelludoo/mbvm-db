package erdosstraus

import (
	"math/big"
	"testing"
)

func TestComposeCRTRegistersDerivedRuleAndCSRRelations(t *testing.T) {
	base := StandardSeedRules()
	composer := NewRuleComposer(base)
	graphBefore := composer.Graph
	initialNodes := len(graphBefore.RuleIDs)
	var mod4, mod3 *Rule
	for _, rule := range base {
		switch rule.ID {
		case "RULE-MOD4-3":
			mod4 = rule
		case "RULE-MOD3-2":
			mod3 = rule
		}
	}
	if mod4 == nil || mod3 == nil {
		t.Fatal("test fixture missing modular seed rules")
	}
	candidate, err := composer.ComposeCRT(mod4, mod3)
	if err != nil {
		t.Fatalf("ComposeCRT failed: %v", err)
	}
	if !candidate.Verified {
		t.Fatalf("candidate failed finite-instance checks: %s", candidate.Refutation)
	}
	if composer.Graph != graphBefore {
		t.Fatal("composer replaced CSR graph pointer, invalidating existing references")
	}
	if len(composer.Graph.RuleIDs) != initialNodes+1 {
		t.Fatalf("new rule node missing: got %d nodes, want %d", len(composer.Graph.RuleIDs), initialNodes+1)
	}
	if len(composer.Graph.RowOffsets) != len(composer.Graph.RuleIDs)+1 {
		t.Fatalf("inconsistent CSR row offsets: %d nodes, %d offsets", len(composer.Graph.RuleIDs), len(composer.Graph.RowOffsets))
	}
	derivedIdx, found := composer.Graph.IDToIdx[candidate.DerivedRule.ID]
	if !found {
		t.Fatalf("derived rule %q missing from CSR index", candidate.DerivedRule.ID)
	}
	if len(composer.BaseRules) != len(base)+1 {
		t.Fatalf("derived rule missing from composer rule catalog")
	}
	hasEdge := func(sourceID, targetID string, relation RelationType) bool {
		sourceIdx := composer.Graph.IDToIdx[sourceID]
		for _, edge := range composer.Graph.Neighbors(sourceIdx) {
			if composer.Graph.RuleIDs[edge.Target] == targetID && edge.Rel == relation {
				return true
			}
		}
		return false
	}
	for _, parentID := range []string{mod4.ID, mod3.ID} {
		if !hasEdge(parentID, candidate.DerivedRule.ID, RelComposition) {
			t.Errorf("composition edge %s -> %s is missing", parentID, candidate.DerivedRule.ID)
		}
		if !hasEdge(candidate.DerivedRule.ID, parentID, RelDependency) {
			t.Errorf("dependency edge %s -> %s is missing", candidate.DerivedRule.ID, parentID)
		}
	}
	if len(composer.Graph.Neighbors(derivedIdx)) < 2 {
		t.Fatal("derived CSR node has no usable prerequisite edges")
	}
	sol, err := candidate.DerivedRule.Apply(big.NewInt(11))
	if err != nil {
		t.Fatalf("derived CRT rule failed on its own class: %v", err)
	}
	if err := VerifySolution(big.NewInt(11), sol); err != nil {
		t.Fatalf("derived rule's numeric solution is invalid: %v", err)
	}
	if _, err := composer.ComposeCRT(mod4, mod3); err == nil {
		t.Fatal("duplicate composition must not overwrite an existing CSR rule ID")
	}
}
