package erdosstraus

import (
	"math/rand"
	"slices"
	"strconv"
	"testing"
)

// This oracle intentionally retains the old O(N^2) nested-pair scan.
// It is not implemented in terms of appendIndexedGeneralizationEdges.
func legacyCSRPairwiseReference(rules []*Rule) *MathCSRGraph {
	n := len(rules)
	idx := make(map[string]int, n)
	ids := make([]string, n)
	for i, r := range rules {
		idx[r.ID] = i
		ids[i] = r.ID
	}
	rows := make([][]GraphEdge, n)
	for i, r := range rules {
		for _, dep := range r.Dependencies {
			if target, ok := idx[dep]; ok {
				rows[i] = append(rows[i], GraphEdge{Target: target, Rel: RelDependency})
			}
		}
	}
	for i, r1 := range rules {
		if r1.Modulus <= 0 {
			continue
		}
		for j, r2 := range rules {
			if i == j || r2.Modulus <= 0 || r1.Modulus%r2.Modulus != 0 {
				continue
			}
			subsumed := true
			for _, residue := range r1.Residues {
				if !slices.Contains(r2.Residues, residue%r2.Modulus) {
					subsumed = false
					break
				}
			}
			if subsumed && len(r1.Residues) > 0 {
				rows[i] = append(rows[i], GraphEdge{Target: j, Rel: RelGeneralizes})
			}
		}
	}
	out := &MathCSRGraph{RuleIDs: ids, IDToIdx: idx, RowOffsets: make([]int, n+1)}
	for i, row := range rows {
		out.RowOffsets[i+1] = out.RowOffsets[i] + len(row)
		for _, edge := range row {
			out.ColIndices = append(out.ColIndices, edge.Target)
			out.RelTypes = append(out.RelTypes, edge.Rel)
		}
	}
	return out
}

func assertIndexedMatchesPairwise(t *testing.T, rules []*Rule) {
	t.Helper()
	actual := NewMathCSRGraph(rules)
	expected := legacyCSRPairwiseReference(rules)
	if !slices.Equal(actual.RuleIDs, expected.RuleIDs) ||
		!slices.Equal(actual.RowOffsets, expected.RowOffsets) ||
		!slices.Equal(actual.ColIndices, expected.ColIndices) ||
		!slices.Equal(actual.RelTypes, expected.RelTypes) {
		t.Fatalf("indexed CSR differs from legacy pairwise oracle; rules=%+v\nindexed offsets=%v columns=%v rel=%v\nlegacy offsets=%v columns=%v rel=%v",
			rules, actual.RowOffsets, actual.ColIndices, actual.RelTypes,
			expected.RowOffsets, expected.ColIndices, expected.RelTypes)
	}
}

func TestCSRModularIndexExactLegacyParity(t *testing.T) {
	fixed := []*Rule{
		{ID: "A", Modulus: 24, Residues: []int64{5, 17}, Dependencies: []string{"B", "B", "UNKNOWN"}},
		{ID: "B", Modulus: 12, Residues: []int64{5}},
		{ID: "C", Modulus: 6, Residues: []int64{5, 5}},
		{ID: "D", Modulus: 24, Residues: []int64{5, 5}},
		{ID: "E", Modulus: 2, Residues: []int64{1}},
		{ID: "F", Modulus: 0, Residues: []int64{1}},
		{ID: "G", Modulus: 8, Residues: nil},
		{ID: "H", Modulus: 24, Residues: []int64{-1, 23}},
		{ID: "I", Modulus: 6, Residues: []int64{-1}},
	}
	assertIndexedMatchesPairwise(t, fixed)

	// Same-modulus, different singleton residues: a known quadratic
	// bottleneck where the index should return only an exact posting.
	unique := make([]*Rule, 2048)
	for i := range unique {
		unique[i] = &Rule{
			ID: "unique-" + strconv.Itoa(i),
			Modulus: 1000003,
			Residues: []int64{int64(i)},
		}
	}
	assertIndexedMatchesPairwise(t, unique)

	// Adversarial equal-residue case: O(N^2) real edges must be emitted.
	equal := make([]*Rule, 64)
	for i := range equal {
		equal[i] = &Rule{ID: "equal-" + strconv.Itoa(i),
			Modulus: 29, Residues: []int64{3, 3}}
	}
	assertIndexedMatchesPairwise(t, equal)
}

func TestCSRModularIndexRandomizedLegacyParity(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	moduli := []int64{0, -3, 1, 2, 3, 4, 6, 8, 9, 12, 18, 24, 36, 72, 127, 1000003}
	for trial := 0; trial < 200; trial++ {
		n := 3 + rng.Intn(57)
		rules := make([]*Rule, n)
		for i := range rules {
			m := moduli[rng.Intn(len(moduli))]
			r := &Rule{ID: "rule-" + strconv.Itoa(i), Modulus: m}
			if i > 0 && rng.Intn(3) == 0 {
				r.Dependencies = append(r.Dependencies, "rule-"+strconv.Itoa(rng.Intn(i)))
			}
			for k := 0; k < rng.Intn(5); k++ {
				value := int64(rng.Intn(25) - 2)
				if m > 0 && rng.Intn(3) == 0 {
					value = int64(rng.Intn(10))%m
				}
				r.Residues = append(r.Residues, value)
			}
			rules[i] = r
		}
		assertIndexedMatchesPairwise(t, rules)
	}
}
