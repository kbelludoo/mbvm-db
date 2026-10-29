package realeval

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// CRTDepthLab tests three-premise CRT intersection with a two-stage
// dependency graph and irrelevant catalog entries. PUBLIC development only:
// neither the search paths nor these cases constitute a blind holdout.
type CRTDepthLab struct {
	Tasks   []CRTDepthTask
	Catalog []*erdosstraus.Rule
	Graph   *erdosstraus.MathCSRGraph
	byID    map[string]*erdosstraus.Rule
}

type CRTDepthTask struct {
	ID        string   `json:"id"`
	GoalID    string   `json:"goal_id"`
	MidID     string   `json:"mid_id"`
	LeafIDs   []string `json:"leaf_ids"`
	NoiseIDs  []string `json:"noise_ids"`
}

type CRTDepthCertificate struct {
	Intermediate *CRTDevelopmentCertificate `json:"intermediate"`
	Final        *CRTDevelopmentCertificate `json:"final"`
}

type CRTDepthArmResult struct {
	Arm             string               `json:"arm"`
	RetrievedIDs    []string             `json:"retrieved_ids"`
	ContextBytes    int                  `json:"context_bytes"`
	RetrievalNanos  int64                `json:"retrieval_nanoseconds"`
	Certificate     *CRTDepthCertificate `json:"certificate,omitempty"`
	Certified       bool                 `json:"certified"`
	Error           string               `json:"error,omitempty"`
}

type CRTDepthPairedResult struct {
	TaskID          string            `json:"task_id"`
	Conventional    CRTDepthArmResult `json:"conventional"`
	CSR             CRTDepthArmResult `json:"csr"`
	SameContext     bool              `json:"same_context"`
	SameCertificate bool              `json:"same_certificate"`
}

// NewCRTDepthLab extends the 12 public paired CRT tasks with a third
// independent premise, an intermediate goal, a final goal and two distractors.
// Dependencies: FINAL -> [MID, L3], MID -> [L1, L2].
// Distractors are not declared as proof dependencies; neither retrieval arm
// receives a privileged hint about which additional rules to ignore.
func NewCRTDepthLab() *CRTDepthLab {
	firstTwo := NewCRTDevelopmentLab()
	third := [][2]int64{
		{5, 3}, {8, 2}, {9, 5}, {8, 7}, {9, 4}, {7, 3},
		{9, 8}, {8, 6}, {9, 7}, {8, 1}, {7, 4}, {5, 3},
	}
	lab := &CRTDepthLab{byID: make(map[string]*erdosstraus.Rule)}
	add := func(r *erdosstraus.Rule) {
		lab.Catalog = append(lab.Catalog, r)
		lab.byID[r.ID] = r
	}
	for i, baseTask := range firstTwo.Tasks {
		l1 := firstTwo.byID[fmt.Sprintf("DEV-CRT-%02d-L1", i+1)]
		l2 := firstTwo.byID[fmt.Sprintf("DEV-CRT-%02d-L2", i+1)]
		mid := firstTwo.byID[baseTask.GoalID]
		l3ID := fmt.Sprintf("DEPTH-%02d-L3", i+1)
		finalID := fmt.Sprintf("DEPTH-%02d-FINAL", i+1)
		l3 := &erdosstraus.Rule{
			ID: l3ID, Name: "Third independent CRT premise",
			Modulus: third[i][0], Residues: []int64{third[i][1]},
			Conclusion: fmt.Sprintf("n = %d (mod %d)", third[i][1], third[i][0]),
		}
		final := &erdosstraus.Rule{
			ID: finalID, Name: "Three-premise unique CRT class",
			Dependencies: []string{mid.ID, l3ID},
			Conclusion: "Derive unique simultaneous residue; answer not stored.",
		}
		add(l1)
		add(l2)
		add(mid)
		add(l3)
		add(final)
		noiseIDs := make([]string, 0, 2)
		for j, parent := range []*erdosstraus.Rule{l1, l3} {
			noiseID := fmt.Sprintf("DEPTH-%02d-NOISE-%d", i+1, j+1)
			d := &erdosstraus.Rule{
				ID: noiseID, Name: "Irrelevant, conflicting modular distractor",
				Modulus: parent.Modulus,
				Residues: []int64{(parent.Residues[0]+1)%parent.Modulus},
				Conclusion: "This congruence is not a dependency of the requested CRT goal.",
			}
			add(d)
			noiseIDs = append(noiseIDs, noiseID)
		}
		lab.Tasks = append(lab.Tasks, CRTDepthTask{
			ID: fmt.Sprintf("DEPTH-%02d", i+1), GoalID: finalID,
			MidID: mid.ID, LeafIDs: []string{l1.ID, l2.ID, l3ID},
			NoiseIDs: noiseIDs,
		})
	}
	lab.Graph = erdosstraus.NewMathCSRGraph(lab.Catalog)
	return lab
}

// retrieveDepthIndex follows explicit rule dependencies from the same
// ID-indexed catalog. It is a strong recursive conventional baseline.
func (l *CRTDepthLab) retrieveDepthIndex(goalID string) ([]*erdosstraus.Rule, error) {
	seen := map[string]bool{}
	var leaves []*erdosstraus.Rule
	var walk func(string) error
	walk = func(id string) error {
		if seen[id] { return nil }
		r, ok := l.byID[id]
		if !ok { return fmt.Errorf("missing conventional rule %q", id) }
		seen[id] = true
		if r.Modulus > 0 { leaves = append(leaves, r) }
		for _, dep := range r.Dependencies {
			if err := walk(dep); err != nil { return err }
		}
		return nil
	}
	if err := walk(goalID); err != nil { return nil, err }
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].ID < leaves[j].ID })
	return leaves, nil
}

// retrieveDepthCSR traverses CSR RelDependency edges only; the same
// other relation types, rules and hypotheses are available to both arms.
func (l *CRTDepthLab) retrieveDepthCSR(goalID string) ([]*erdosstraus.Rule, error) {
	root, ok := l.Graph.IDToIdx[goalID]
	if !ok { return nil, fmt.Errorf("missing CSR goal %q", goalID) }
	seen := map[int]bool{}
	var leaves []*erdosstraus.Rule
	var walk func(int) error
	walk = func(idx int) error {
		if seen[idx] { return nil }
		if idx < 0 || idx >= len(l.Catalog) { return fmt.Errorf("invalid CSR index %d", idx) }
		seen[idx] = true
		r := l.Catalog[idx]
		if r.Modulus > 0 { leaves = append(leaves, r) }
		for _, edge := range l.Graph.Neighbors(idx) {
			if edge.Rel == erdosstraus.RelDependency {
				if err := walk(edge.Target); err != nil { return err }
			}
		}
		return nil
	}
	if err := walk(root); err != nil { return nil, err }
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].ID < leaves[j].ID })
	return leaves, nil
}

// composeDepth uses the SAME exact CRT executor and checker for both arms.
// A returned certificate includes MID=L1+L2 and FINAL=MID+L3.
func (l *CRTDepthLab) composeDepth(task CRTDepthTask, leaves []*erdosstraus.Rule) (*CRTDepthCertificate, error) {
	if len(leaves) != 3 { return nil, fmt.Errorf("requires exactly 3 retrieved CRT premises, got %d", len(leaves)) }
	found := make(map[string]*erdosstraus.Rule, len(leaves))
	for _, r := range leaves { found[r.ID] = r }
	l1, l2, l3 := found[task.LeafIDs[0]], found[task.LeafIDs[1]], found[task.LeafIDs[2]]
	if l1 == nil || l2 == nil || l3 == nil { return nil, fmt.Errorf("missing a required CRT leaf") }
	stageOne := []*erdosstraus.Rule{l1, l2}
	sort.Slice(stageOne, func(i, j int) bool { return stageOne[i].ID < stageOne[j].ID })
	midCert, err := composeCRTDevelopment(stageOne)
	if err != nil { return nil, fmt.Errorf("compose intermediate CRT: %w", err) }
	if err := verifyCRTDevelopment(l.byID[task.MidID], midCert, l.byID); err != nil {
		return nil, fmt.Errorf("verify intermediate CRT: %w", err)
	}
	mod, okM := new(big.Int).SetString(midCert.Modulus, 10)
	res, okR := new(big.Int).SetString(midCert.Residue, 10)
	if !okM || !okR || !mod.IsInt64() || !res.IsInt64() {
		return nil, fmt.Errorf("invalid intermediate CRT class")
	}
	derivedMid := &erdosstraus.Rule{ID: task.MidID, Modulus: mod.Int64(), Residues: []int64{res.Int64()}}
	stageTwo := []*erdosstraus.Rule{derivedMid, l3}
	sort.Slice(stageTwo, func(i, j int) bool { return stageTwo[i].ID < stageTwo[j].ID })
	finalCert, err := composeCRTDevelopment(stageTwo)
	if err != nil { return nil, fmt.Errorf("compose final CRT: %w", err) }
	// Independent certificate verification must explicitly bind the symbolic
	// intermediate node to its previously validated arithmetic result.
	byID := make(map[string]*erdosstraus.Rule, len(l.byID))
	for k, v := range l.byID { byID[k] = v }
	byID[task.MidID] = derivedMid
	if err := verifyCRTDevelopment(l.byID[task.GoalID], finalCert, byID); err != nil {
		return nil, fmt.Errorf("verify final CRT: %w", err)
	}
	// Explicitly check the final residue against all THREE original premises.
	overall, ok := new(big.Int).SetString(finalCert.Residue, 10)
	if !ok { return nil, fmt.Errorf("invalid final residue") }
	for _, id := range task.LeafIDs {
		r := found[id]
		if new(big.Int).Mod(overall, big.NewInt(r.Modulus)).Int64() != r.Residues[0] {
			return nil, fmt.Errorf("final residue fails original premise %s", id)
		}
	}
	return &CRTDepthCertificate{Intermediate: midCert, Final: finalCert}, nil
}

func sameDepthCertificates(a, b *CRTDepthCertificate) bool {
	if a == nil || b == nil || a.Intermediate == nil || b.Intermediate == nil || a.Final == nil || b.Final == nil { return false }
	return a.Intermediate.Residue == b.Intermediate.Residue &&
		a.Intermediate.Modulus == b.Intermediate.Modulus &&
		a.Final.Residue == b.Final.Residue && a.Final.Modulus == b.Final.Modulus &&
		strings.Join(a.Intermediate.ParentIDs, ",") == strings.Join(b.Intermediate.ParentIDs, ",") &&
		strings.Join(a.Final.ParentIDs, ",") == strings.Join(b.Final.ParentIDs, ",")
}

// RunDepthPaired is a local parity control, NOT a retrieval superiority test.
// Times are single-call diagnostic measurements, not comparable benchmarks.
func (l *CRTDepthLab) RunDepthPaired() ([]CRTDepthPairedResult, error) {
	results := make([]CRTDepthPairedResult, 0, len(l.Tasks))
	for _, task := range l.Tasks {
		run := func(name string, retrieve func(string) ([]*erdosstraus.Rule, error)) (CRTDepthArmResult, string) {
			out := CRTDepthArmResult{Arm: name}
			start := time.Now()
			rules, err := retrieve(task.GoalID)
			out.RetrievalNanos = time.Since(start).Nanoseconds()
			if err != nil { out.Error = err.Error(); return out, "" }
			context := formatCRTPremises(rules)
			out.ContextBytes = len(context)
			for _, r := range rules { out.RetrievedIDs = append(out.RetrievedIDs, r.ID) }
			out.Certificate, err = l.composeDepth(task, rules)
			if err != nil { out.Error = err.Error() } else { out.Certified = true }
			return out, context
		}
		b, cb := run("B: conventional multihop", l.retrieveDepthIndex)
		c, cc := run("C: CSR multihop", l.retrieveDepthCSR)
		record := CRTDepthPairedResult{
			TaskID: task.ID, Conventional: b, CSR: c,
			SameContext: cb == cc, SameCertificate: sameDepthCertificates(b.Certificate, c.Certificate),
		}
		results = append(results, record)
		if !b.Certified || !c.Certified || !record.SameContext || !record.SameCertificate {
			return results, fmt.Errorf("depth retrieval parity failed for %s: B=%s C=%s", task.ID, b.Error, c.Error)
		}
	}
	return results, nil
}
