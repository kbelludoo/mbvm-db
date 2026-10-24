package erdosstraus

import "testing"

// A second CRT composition must not erase the RelComposition edges that
// connected earlier derived rules. The CSR catalog is rebuilt each time.
func TestComposeCRTPreservesPreviousCompositionEdges(t *testing.T) {
	base := StandardSeedRules()
	composer := NewRuleComposer(base)
	originalGraph := composer.Graph

	var mod4, mod3, mod8 *Rule
	for _, r := range base {
		switch r.ID {
		case "RULE-MOD4-3":
			mod4 = r
		case "RULE-MOD3-2":
			mod3 = r
		case "RULE-MOD8-5":
			mod8 = r
		}
	}
	if mod4 == nil || mod3 == nil || mod8 == nil {
		t.Fatal("missing modular development rules")
	}

	// These are independent CRT intersections, not a novel proof of
	// Erdos-Straus. The test checks persistence of the composition graph.
	first, err := composer.ComposeCRT(mod4, mod3) // n = 11 (mod 12)
	if err != nil || first == nil || !first.Verified {
		t.Fatalf("first CRT composition failed: candidate=%+v err=%v", first, err)
	}
	second, err := composer.ComposeCRT(mod3, mod8) // n = 5 (mod 24)
	if err != nil || second == nil || !second.Verified {
		t.Fatalf("second CRT composition failed: candidate=%+v err=%v", second, err)
	}

	if composer.Graph != originalGraph {
		t.Fatal("CRT composition replaced the graph pointer")
	}
	if len(composer.Graph.RuleIDs) != len(base)+2 {
		t.Fatalf("wanted %d nodes, found %d", len(base)+2, len(composer.Graph.RuleIDs))
	}
	if len(composer.Graph.RowOffsets) != len(composer.Graph.RuleIDs)+1 {
		t.Fatal("inconsistent CSR row offsets after successive compositions")
	}

	edgeExists := func(fromID, toID string, rel RelationType) bool {
		from, found := composer.Graph.IDToIdx[fromID]
		if !found {
			t.Errorf("source node %q not registered", fromID)
			return false
		}
		if _, found := composer.Graph.IDToIdx[toID]; !found {
			t.Errorf("target node %q not registered", toID)
			return false
		}
		for _, edge := range composer.Graph.Neighbors(from) {
			if composer.Graph.RuleIDs[edge.Target] == toID && edge.Rel == rel {
				return true
			}
		}
		return false
	}

	for _, pair := range []struct {
		parent string
		derived string
	}{
		{mod4.ID, first.DerivedRule.ID},
		{mod3.ID, first.DerivedRule.ID},
		{mod3.ID, second.DerivedRule.ID},
		{mod8.ID, second.DerivedRule.ID},
	} {
		if !edgeExists(pair.parent, pair.derived, RelComposition) {
			t.Errorf("missing parent->derived RelComposition: %s -> %s", pair.parent, pair.derived)
		}
		if !edgeExists(pair.derived, pair.parent, RelDependency) {
			t.Errorf("missing derived->parent RelDependency: %s -> %s", pair.derived, pair.parent)
		}
	}
}
