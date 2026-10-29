package realeval

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// CRTDevelopmentLab is a PUBLIC development-only comparison of two retrieval
// backends. It is not a blind benchmark and does not call an LLM.
// Both backends share the same catalog, CRT executor, proof checker and formatter.
type CRTDevelopmentLab struct {
	Tasks   []CRTDevelopmentTask
	Catalog []*erdosstraus.Rule
	byID    map[string]*erdosstraus.Rule
	graph   *erdosstraus.MathCSRGraph
}
type CRTDevelopmentTask struct {
	ID     string `json:"id"`
	GoalID string `json:"goal_id"`
}
type CRTDevelopmentCertificate struct {
	Residue   string   `json:"residue"`
	Modulus   string   `json:"modulus"`
	ParentIDs []string `json:"parent_ids"`
}
type CRTRetrievalTrial struct {
	TaskID               string                     `json:"task_id"`
	Arm                  string                     `json:"arm"`
	RetrievedIDs         []string                   `json:"retrieved_ids"`
	ContextBytes         int                        `json:"context_bytes"`
	RetrievalNanoseconds int64                      `json:"retrieval_nanoseconds"`
	Certificate          *CRTDevelopmentCertificate `json:"certificate,omitempty"`
	Certified            bool                       `json:"certified"`
	Error                string                     `json:"error,omitempty"`
}
type CRTPairedTrial struct {
	TaskID          string            `json:"task_id"`
	Conventional    CRTRetrievalTrial `json:"conventional"`
	CSR             CRTRetrievalTrial `json:"csr"`
	SameContext     bool              `json:"same_context"`
	SameCertificate bool              `json:"same_certificate"`
}

// NewCRTDevelopmentLab builds 12 public modular-intersection fixtures.
// Goal nodes encode dependencies but never contain the answer.
func NewCRTDevelopmentLab() *CRTDevelopmentLab {
	fixtures := [][4]int64{
		{8, 5, 9, 7}, {5, 3, 7, 4}, {7, 2, 11, 3},
		{11, 6, 13, 8}, {13, 5, 17, 12}, {9, 4, 16, 13},
		{16, 9, 25, 21}, {17, 7, 19, 8}, {19, 13, 23, 18},
		{23, 10, 29, 22}, {25, 6, 27, 14}, {27, 11, 32, 17},
	}
	lab := &CRTDevelopmentLab{byID: make(map[string]*erdosstraus.Rule)}
	for i, v := range fixtures {
		p1, p2, goal := fmt.Sprintf("DEV-CRT-%02d-L1", i+1), fmt.Sprintf("DEV-CRT-%02d-L2", i+1), fmt.Sprintf("DEV-CRT-%02d-GOAL", i+1)
		r1 := &erdosstraus.Rule{ID: p1, Name: "CRT premise", Modulus: v[0], Residues: []int64{v[1]}, Conclusion: fmt.Sprintf("n = %d (mod %d)", v[1], v[0])}
		r2 := &erdosstraus.Rule{ID: p2, Name: "CRT premise", Modulus: v[2], Residues: []int64{v[3]}, Conclusion: fmt.Sprintf("n = %d (mod %d)", v[3], v[2])}
		g := &erdosstraus.Rule{ID: goal, Name: "Unique simultaneous CRT class", Dependencies: []string{p1, p2}, Conclusion: "Derive the unique simultaneous residue modulo the product; do not assume the answer."}
		lab.Catalog = append(lab.Catalog, r1, r2, g)
		lab.Tasks = append(lab.Tasks, CRTDevelopmentTask{ID: fmt.Sprintf("DEV-CRT-%02d", i+1), GoalID: goal})
		lab.byID[p1], lab.byID[p2], lab.byID[goal] = r1, r2, g
	}
	lab.graph = erdosstraus.NewMathCSRGraph(lab.Catalog)
	return lab
}

// retrieveIndex is a strong conventional baseline: indexed ID lookup with
// recursive dependency expansion, using the same metadata as CSR.
func (lab *CRTDevelopmentLab) retrieveIndex(goalID string) ([]*erdosstraus.Rule, error) {
	visited := make(map[string]bool)
	var result []*erdosstraus.Rule
	var walk func(string) error
	walk = func(id string) error {
		if visited[id] {
			return nil
		}
		visited[id] = true
		r, ok := lab.byID[id]
		if !ok {
			return fmt.Errorf("unknown rule %q", id)
		}
		if r.Modulus > 0 {
			result = append(result, r)
		}
		for _, dep := range r.Dependencies {
			if err := walk(dep); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(goalID); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// retrieveCSR traverses the actual CSR adjacency arrays via Neighbors,
// not the Rule.Dependencies lists.
func (lab *CRTDevelopmentLab) retrieveCSR(goalID string) ([]*erdosstraus.Rule, error) {
	root, ok := lab.graph.IDToIdx[goalID]
	if !ok {
		return nil, fmt.Errorf("unknown CSR rule %q", goalID)
	}
	visited := make(map[int]bool)
	var result []*erdosstraus.Rule
	var walk func(int) error
	walk = func(idx int) error {
		if visited[idx] {
			return nil
		}
		visited[idx] = true
		if idx < 0 || idx >= len(lab.Catalog) {
			return fmt.Errorf("invalid CSR index %d", idx)
		}
		r := lab.Catalog[idx]
		if r.Modulus > 0 {
			result = append(result, r)
		}
		for _, edge := range lab.graph.Neighbors(idx) {
			if edge.Rel == erdosstraus.RelDependency {
				if err := walk(edge.Target); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func formatCRTPremises(rules []*erdosstraus.Rule) string {
	var b strings.Builder
	for _, r := range rules {
		fmt.Fprintf(&b, "%s: %s\n", r.ID, r.Conclusion)
	}
	return b.String()
}

// composeCRTDevelopment is the EXACT SAME composer in both arms; it constructs
// only a modular class, never an Erdos-Straus solution or universal theorem.
func composeCRTDevelopment(rules []*erdosstraus.Rule) (*CRTDevelopmentCertificate, error) {
	if len(rules) != 2 {
		return nil, fmt.Errorf("CRT needs exactly two independent premises, got %d", len(rules))
	}
	a, b := rules[0], rules[1]
	if a.Modulus <= 1 || b.Modulus <= 1 || len(a.Residues) != 1 || len(b.Residues) != 1 || a.Residues[0] < 0 || b.Residues[0] < 0 || a.Residues[0] >= a.Modulus || b.Residues[0] >= b.Modulus {
		return nil, fmt.Errorf("invalid CRT premises")
	}
	m1, m2 := big.NewInt(a.Modulus), big.NewInt(b.Modulus)
	if new(big.Int).GCD(nil, nil, m1, m2).Cmp(big.NewInt(1)) != 0 {
		return nil, fmt.Errorf("CRT premises not coprime")
	}
	inv := new(big.Int).ModInverse(m1, m2)
	if inv == nil {
		return nil, fmt.Errorf("modular inverse absent")
	}
	k := new(big.Int).Sub(big.NewInt(b.Residues[0]), big.NewInt(a.Residues[0]))
	k.Mul(k, inv)
	k.Mod(k, m2)
	M := new(big.Int).Mul(m1, m2)
	R := new(big.Int).Mul(k, m1)
	R.Add(R, big.NewInt(a.Residues[0]))
	R.Mod(R, M)
	return &CRTDevelopmentCertificate{Residue: R.String(), Modulus: M.String(), ParentIDs: []string{a.ID, b.ID}}, nil
}

// verifyCRTDevelopment performs a separate exact arithmetic check and checks
// that both coprime nontrivial moduli are needed for uniqueness modulo M.
func verifyCRTDevelopment(goal *erdosstraus.Rule, cert *CRTDevelopmentCertificate, byID map[string]*erdosstraus.Rule) error {
	if goal == nil || cert == nil || len(goal.Dependencies) != 2 || len(cert.ParentIDs) != 2 {
		return fmt.Errorf("missing CRT goal or parents")
	}
	ids := append([]string(nil), goal.Dependencies...)
	sort.Strings(ids)
	if cert.ParentIDs[0] != ids[0] || cert.ParentIDs[1] != ids[1] {
		return fmt.Errorf("CRT certificate parent IDs do not match the goal")
	}
	a, b := byID[ids[0]], byID[ids[1]]
	if a == nil || b == nil || a.Modulus <= 1 || b.Modulus <= 1 || len(a.Residues) != 1 || len(b.Residues) != 1 {
		return fmt.Errorf("invalid premise nodes")
	}
	m1, m2 := big.NewInt(a.Modulus), big.NewInt(b.Modulus)
	if new(big.Int).GCD(nil, nil, m1, m2).Cmp(big.NewInt(1)) != 0 {
		return fmt.Errorf("parents are not coprime")
	}
	M, okM := new(big.Int).SetString(cert.Modulus, 10)
	R, okR := new(big.Int).SetString(cert.Residue, 10)
	if !okM || !okR {
		return fmt.Errorf("invalid certificate integers")
	}
	if M.Cmp(new(big.Int).Mul(m1, m2)) != 0 || R.Sign() < 0 || R.Cmp(M) >= 0 {
		return fmt.Errorf("incorrect canonical CRT class")
	}
	if new(big.Int).Mod(R, m1).Int64() != a.Residues[0] || new(big.Int).Mod(R, m2).Int64() != b.Residues[0] {
		return fmt.Errorf("incorrect CRT residues")
	}
	// Both moduli >1 and coprime: each premise alone admits >1 classes mod M.
	return nil
}

// RunCRTDevelopmentPaired evaluates 12 public CRT tasks with the same
// composer, checker and formatter for conventional index (B) and CSR (C).
// Retrieval timings are diagnostic, not stable latency benchmarks.
func (lab *CRTDevelopmentLab) RunCRTDevelopmentPaired() ([]CRTPairedTrial, error) {
	results := make([]CRTPairedTrial, 0, len(lab.Tasks))
	for _, task := range lab.Tasks {
		goal := lab.byID[task.GoalID]
		run := func(arm string, retrieve func(string) ([]*erdosstraus.Rule, error)) (CRTRetrievalTrial, string) {
			t := CRTRetrievalTrial{TaskID: task.ID, Arm: arm}
			started := time.Now()
			rules, err := retrieve(task.GoalID)
			t.RetrievalNanoseconds = time.Since(started).Nanoseconds()
			if err != nil {
				t.Error = err.Error()
				return t, ""
			}
			context := formatCRTPremises(rules)
			t.ContextBytes = len(context)
			for _, r := range rules {
				t.RetrievedIDs = append(t.RetrievedIDs, r.ID)
			}
			t.Certificate, err = composeCRTDevelopment(rules)
			if err == nil {
				err = verifyCRTDevelopment(goal, t.Certificate, lab.byID)
			}
			if err != nil {
				t.Error = err.Error()
			} else {
				t.Certified = true
			}
			return t, context
		}
		b, bText := run("B: conventional indexed expansion", lab.retrieveIndex)
		c, cText := run("C: CSR dependency traversal", lab.retrieveCSR)
		equalCertificate := b.Certificate != nil && c.Certificate != nil && b.Certificate.Residue == c.Certificate.Residue && b.Certificate.Modulus == c.Certificate.Modulus && strings.Join(b.Certificate.ParentIDs, ",") == strings.Join(c.Certificate.ParentIDs, ",")
		trial := CRTPairedTrial{TaskID: task.ID, Conventional: b, CSR: c, SameContext: bText == cText, SameCertificate: equalCertificate}
		results = append(results, trial)
		if !b.Certified || !c.Certified || !trial.SameContext || !trial.SameCertificate {
			return results, fmt.Errorf("CRT retrieval parity failure in %s: B=%s C=%s", task.ID, b.Error, c.Error)
		}
	}
	return results, nil
}
