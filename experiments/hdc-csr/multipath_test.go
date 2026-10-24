package hdccsr

import (
    "encoding/json"
    "math/rand"
    "os"
    "sort"
    "testing"
)

// These fixtures are synthetic; they test associative capacity, NOT trained semantic generalization.
const multipathSeeds = 50
const multipathNodes = 160
const multipathClusters = 4

type multipathFixture struct {
    graph *ExpCSRGraph
    vectors []Hypervector
    prototypes []Hypervector
    class []int
}

func buildMultipathFixture(seed int64, nodes int) multipathFixture {
    rng := rand.New(rand.NewSource(seed))
    prototypes := make([]Hypervector, multipathClusters)
    for c := range prototypes { prototypes[c] = GenerateRandomHypervector(rng) }
    vectors := make([]Hypervector, nodes)
    classes := make([]int, nodes)
    for v := range vectors {
        classes[v] = rng.Intn(multipathClusters)
        for w := 0; w < WordsPerVector; w++ {
            // Approximately 3% independent bit noise, preserving synthetic class identity.
            mask := rng.Uint64() & rng.Uint64() & rng.Uint64() & rng.Uint64() & rng.Uint64()
            vectors[v][w] = prototypes[classes[v]][w] ^ mask
        }
    }
    adj := make([][]AdjEdge, nodes)
    for v := 0; v < nodes; v++ {
        degree := 3 + rng.Intn(3)
        for j := 0; j < degree; j++ {
            target := rng.Intn(nodes - 1)
            if target >= v { target++ }
            rel := RelCall
            p := rng.Intn(10)
            if p == 8 { rel = RelRevCall }
            if p == 9 { rel = RelMemberOf }
            adj[v] = append(adj[v], AdjEdge{Target:uint16(target), Rel:rel})
        }
    }
    // Inject two-way cycles. Backtracking is excluded by PropagateHop2.
    for v := 0; v+1 < nodes; v += 29 {
        adj[v] = append(adj[v], AdjEdge{Target:uint16(v+1), Rel:RelCall})
        adj[v+1] = append(adj[v+1], AdjEdge{Target:uint16(v), Rel:RelRevCall})
    }
    return multipathFixture{BuildCSRFromAdj(nodes, adj), vectors, prototypes, classes}
}

// Exact two-hop relation filter; do not use this result to score the HDC candidates.
func csrTwoHopCallTargets(g *ExpCSRGraph, source int) []int {
    seen := make(map[int]bool)
    for e := g.Offsets[source]; e < g.Offsets[source+1]; e++ {
        if g.Relations[e] != RelCall { continue }
        u := int(g.Edges[e])
        for f := g.Offsets[u]; f < g.Offsets[u+1]; f++ {
            if g.Relations[f] != RelCall { continue }
            w := int(g.Edges[f])
            if w != source { seen[w] = true }
        }
    }
    result := make([]int, 0, len(seen))
    for id := range seen { result = append(result, id) }
    sort.Ints(result)
    return result
}

type multipathCandidate struct { id int; score float64 }

func rankMultipath(candidates []int, scorer func(int) float64) []int {
    entries := make([]multipathCandidate, 0, len(candidates))
    for _, id := range candidates { entries = append(entries, multipathCandidate{id,scorer(id)}) }
    sort.Slice(entries,func(i,j int)bool{
        if entries[i].score != entries[j].score { return entries[i].score > entries[j].score }
        return entries[i].id < entries[j].id
    })
    out := make([]int, len(entries))
    for i,e := range entries { out[i]=e.id }
    return out
}

func reciprocalRank(ranked []int, relevant map[int]bool) float64 {
    for rank,id := range ranked { if relevant[id] { return 1.0/float64(rank+1) } }
    return 0
}

type multiPathReport struct {
    Seeds int `json:"seeds"`
    Queries int `json:"queries"`
    SkippedNoRelevant int `json:"skipped_no_relevant"`
    AverageRelevant float64 `json:"average_relevant"`
    MeanCSRRecall10 float64 `json:"csr_semantic_recall_at_10"`
    MeanCSRMRR float64 `json:"csr_semantic_mrr"`
    MeanSemanticRecall10 float64 `json:"semantic_only_recall_at_10"`
    MeanSemanticMRR float64 `json:"semantic_only_mrr"`
    MeanHDCRecall10 float64 `json:"hdc_hybrid_recall_at_10"`
    MeanHDCMRR float64 `json:"hdc_hybrid_mrr"`
    Evaluation string `json:"evaluation"`
}

func TestMultiPath50Seeds(t *testing.T) {
    report := multiPathReport{Seeds:multipathSeeds,Evaluation:"synthetic_context_capacity_not_neural_generalization"}
    // Independent seeds, a query for each of three anchor nodes per seed.
    for s := 0; s < multipathSeeds; s++ {
        f := buildMultipathFixture(int64(50000+s),multipathNodes)
        system := NewHDCSystem(42)
        hop1 := system.PropagateHop1(f.graph, f.vectors)
        hop2 := system.PropagateHop2(f.graph, hop1)
        cfg := system.RelConfigs[RelCall]
        role := Bind(RotL8K(cfg.RoleVector,cfg.Shift),cfg.RoleVector)
        all := make([]int,0,multipathNodes)
        for i:=0;i<multipathNodes;i++ { all = append(all,i) }
        for _,source := range []int{0,1,2} {
            exactTargets := csrTwoHopCallTargets(f.graph,source)
            // Class is a query filter, not an input to structural HDC propagation.
            desiredClass := (s+source)%multipathClusters
            relevant:=make(map[int]bool)
            for _,id:=range exactTargets { if f.class[id]==desiredClass { relevant[id]=true } }
            if len(relevant)==0 { report.SkippedNoRelevant++; continue }
            report.Queries++
            report.AverageRelevant+=float64(len(relevant))
            prototype:=f.prototypes[desiredClass]
            baseline:=rankMultipath(exactTargets,func(id int)float64{
                return scoreSemanticOnly(prototype,f.vectors[id])
            })
            // Compute query context from PRECOMPUTED hop2; no graph traversal in HDC query.
            recovered:=RotR8K(Unbind(hop2.Contextual[source],role),2*cfg.Shift)
            semanticOnly:=rankMultipath(all,func(id int)float64{
                if id==source {return -1}
                return scoreSemanticOnly(prototype,f.vectors[id])
            })
            hybrid:=rankMultipath(all,func(id int)float64{
                if id==source {return -1}
                sem:=scoreSemanticOnly(prototype,f.vectors[id])
                structural:=scoreSemanticOnly(recovered,f.vectors[id])
                return 0.45*sem+0.55*structural
            })
            report.MeanCSRRecall10+=evaluateRecallAtK(baseline,relevant,10)
            report.MeanCSRMRR+=reciprocalRank(baseline,relevant)
            report.MeanSemanticRecall10+=evaluateRecallAtK(semanticOnly,relevant,10)
            report.MeanSemanticMRR+=reciprocalRank(semanticOnly,relevant)
            report.MeanHDCRecall10+=evaluateRecallAtK(hybrid,relevant,10)
            report.MeanHDCMRR+=reciprocalRank(hybrid,relevant)
        }
    }
    if report.Queries == 0 { t.Fatal("no relevant multi-path queries generated") }
    q:=float64(report.Queries)
    report.AverageRelevant/=q
    report.MeanCSRRecall10/=q
    report.MeanCSRMRR/=q
    report.MeanSemanticRecall10/=q
    report.MeanSemanticMRR/=q
    report.MeanHDCRecall10/=q
    report.MeanHDCMRR/=q
    if report.MeanCSRMRR < 0 || report.MeanCSRMRR > 1 ||
        report.MeanHDCRecall10 < 0 || report.MeanHDCRecall10 > 1 {
        t.Fatal("invalid evaluation metrics")
    }
    buf,err:=json.MarshalIndent(report,"","  ")
    if err!=nil { t.Fatal(err) }
    if err=os.WriteFile("multipath_quality.json",buf,0644);err!=nil { t.Fatal(err) }
    t.Logf("50 seeds; %d queries; %d skipped, avg relevant %.2f",report.Queries,report.SkippedNoRelevant,report.AverageRelevant)
    t.Logf("Exact CSR+semantic Recall@10 %.3f MRR %.3f", report.MeanCSRRecall10,report.MeanCSRMRR)
    t.Logf("Semantic-only Recall@10 %.3f MRR %.3f", report.MeanSemanticRecall10,report.MeanSemanticMRR)
    t.Logf("HDC precomputed-context hybrid Recall@10 %.3f MRR %.3f",report.MeanHDCRecall10,report.MeanHDCMRR)
}
