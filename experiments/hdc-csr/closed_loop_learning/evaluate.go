package closedloop

import (
	"math/rand/v2"
	"slices"
	"strings"
)

type ranked struct {
	id    string
	score float64
}
type scorer func(Relation, [featureCount]float64) float64

func (c *Corpus) rank(caller string, candidates []string, score scorer) []ranked {
	out := make([]ranked, 0, len(candidates))
	a := c.Symbols[c.byID[caller]]
	for _, id := range candidates {
		b := c.Symbols[c.byID[id]]
		out = append(out, ranked{id, score(Relation{caller, id}, features(a, b))})
	}
	slices.SortFunc(out, func(a, b ranked) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}
		return strings.Compare(a.id, b.id)
	})
	return out
}

type QueryMetric struct {
	Caller    string  `json:"caller"`
	File      string  `json:"file"`
	Relevant  int     `json:"relevant"`
	Recall    float64 `json:"recall_at_k"`
	Precision float64 `json:"precision_at_k"`
	MRR       float64 `json:"mrr"`
	AP        float64 `json:"average_precision"`
}

type Metrics struct {
	EligibleQueries      int           `json:"eligible_queries"`
	ExcludedEmptyQueries int           `json:"excluded_empty_queries"`
	CandidatesPerQuery   int           `json:"candidates_per_query"`
	Recall               float64       `json:"recall_at_k"`
	Precision            float64       `json:"precision_at_k"`
	MRR                  float64       `json:"mrr"`
	MAP                  float64       `json:"map"`
	Rows                 []QueryMetric `json:"-"`
}

func (c *Corpus) evaluate(queries, candidates []string, k int, score scorer) Metrics {
	m := Metrics{CandidatesPerQuery: len(candidates)}
	for _, caller := range queries {
		q := QueryMetric{Caller: caller, File: c.Symbols[c.byID[caller]].File}
		for _, id := range candidates {
			if c.edges[Relation{caller, id}] {
				q.Relevant++
			}
		}
		if q.Relevant == 0 {
			m.ExcludedEmptyQueries++
			continue
		}
		found, atK := 0, 0
		for i, r := range c.rank(caller, candidates, score) {
			if !c.edges[Relation{caller, r.id}] {
				continue
			}
			found++
			if i < k {
				atK++
			}
			if q.MRR == 0 {
				q.MRR = 1 / float64(i+1)
			}
			q.AP += float64(found) / float64(i+1)
		}
		q.Recall = float64(atK) / float64(q.Relevant)
		q.Precision = float64(atK) / float64(min(k, len(candidates)))
		q.AP /= float64(q.Relevant)
		m.Rows = append(m.Rows, q)
		m.Recall += q.Recall
		m.Precision += q.Precision
		m.MRR += q.MRR
		m.MAP += q.AP
	}
	m.EligibleQueries = len(m.Rows)
	if n := float64(m.EligibleQueries); n > 0 {
		m.Recall /= n
		m.Precision /= n
		m.MRR /= n
		m.MAP /= n
	}
	return m
}

type Gain struct {
	Eligible           bool    `json:"eligible"`
	RecallDelta        float64 `json:"recall_delta"`
	MRRDelta           float64 `json:"mrr_delta"`
	ImprovedQueries    int     `json:"improved_recall_queries"`
	WorseQueries       int     `json:"worse_recall_queries"`
	FileBootstrapLower float64 `json:"file_bootstrap_95_lower"`
	FileBootstrapUpper float64 `json:"file_bootstrap_95_upper"`
}

func pairedGain(before, after Metrics, seed int64) Gain {
	g := Gain{Eligible: before.EligibleQueries > 0 && before.EligibleQueries == after.EligibleQueries, RecallDelta: after.Recall - before.Recall, MRRDelta: after.MRR - before.MRR}
	if !g.Eligible {
		return g
	}
	groups := map[string][]float64{}
	files := []string{}
	for i, a := range after.Rows {
		if a.Caller != before.Rows[i].Caller {
			g.Eligible = false
			return g
		}
		delta := a.Recall - before.Rows[i].Recall
		if delta > 0 {
			g.ImprovedQueries++
		}
		if delta < 0 {
			g.WorseQueries++
		}
		if _, ok := groups[a.File]; !ok {
			files = append(files, a.File)
		}
		groups[a.File] = append(groups[a.File], delta)
	}
	slices.Sort(files)
	rng := rand.New(rand.NewPCG(uint64(seed), 73))
	draws := make([]float64, 500)
	for i := range draws {
		sum, n := 0.0, 0
		for range files {
			for _, delta := range groups[files[rng.IntN(len(files))]] {
				sum += delta
				n++
			}
		}
		draws[i] = sum / float64(n)
	}
	slices.Sort(draws)
	g.FileBootstrapLower = draws[12]
	g.FileBootstrapUpper = draws[487]
	return g
}
