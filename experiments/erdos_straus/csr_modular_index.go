package erdosstraus

import (
	"slices"
	"sort"
)

// appendIndexedGeneralizationEdges preserves the old generalization test:
//
//     r1.Modulus % r2.Modulus == 0
//     all r1 residues, reduced modulo r2.Modulus, belong to r2.Residues
//
// Instead of checking every (r1,r2) pair, index rules by their modulus and
// explicitly listed residues. The FIRST residue of r1 is a necessary
// condition for any matching r2; remaining residues are still checked
// exactly as before. No probabilistic approximation or semantic change.
//
// Dependencies precede generalization edges, and matching targets are
// appended in original rule-index order, matching the previous nested loop.
func appendIndexedGeneralizationEdges(rules []*Rule, adj [][]GraphEdge) {
	// modulus -> residue -> original rule indices, always increasing
	postings := make(map[int64]map[int64][]int)
	moduli := make([]int64, 0)
	for j, r := range rules {
		if r.Modulus <= 0 || len(r.Residues) == 0 {
			continue
		}
		byResidue, exists := postings[r.Modulus]
		if !exists {
			byResidue = make(map[int64][]int)
			postings[r.Modulus] = byResidue
			moduli = append(moduli, r.Modulus)
		}
		for _, residue := range r.Residues {
			list := byResidue[residue]
			// Ignore duplicated residues within a rule when building postings.
			// Their old semantics are set membership, not duplicate edges.
			if len(list) == 0 || list[len(list)-1] != j {
				byResidue[residue] = append(list, j)
			}
		}
	}

	for i, r1 := range rules {
		if r1.Modulus <= 0 || len(r1.Residues) == 0 {
			continue
		}
		candidates := make([]int, 0)
		for _, modulus := range moduli {
			if r1.Modulus%modulus != 0 {
				continue
			}
			// Any r2 covering ALL r1 residues must cover this first one.
			first := r1.Residues[0] % modulus
			for _, j := range postings[modulus][first] {
				if i == j {
					continue
				}
				covered := true
				for _, residue := range r1.Residues {
					if !slices.Contains(rules[j].Residues, residue%modulus) {
						covered = false
						break
					}
				}
				if covered {
					candidates = append(candidates, j)
				}
			}
		}
		// The legacy implementation iterates j in original input order,
		// not modulus-group order. Retain that ordering for stable CSR.
		sort.Ints(candidates)
		for _, j := range candidates {
			adj[i] = append(adj[i], GraphEdge{
				Target: j, Rel: RelGeneralizes,
			})
		}
	}
}
