package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "math/rand"
    "os"
    "path/filepath"
    "sort"
    "time"
    hdccsr "github.com/kbelludoo/mbvm-db/experiments/hdc-csr"
)

type metric struct {
    Name string `json:"name"`
    Queries int `json:"queries"`
    RecallAt10 float64 `json:"recall_at_10"`
    PrecisionAt10 float64 `json:"precision_at_10"`
    PrecisionOfReturned float64 `json:"precision_of_returned"`
    MRR float64 `json:"mrr"`
    AvgFalsePositives float64 `json:"avg_false_positives"`
    P50US float64 `json:"p50_latency_us"`
    P95US float64 `json:"p95_latency_us"`
    MeanUS float64 `json:"mean_latency_us"`
}

func exactCall2Targets(g *hdccsr.ExpCSRGraph, src int)map[uint16]bool {
    result:=make(map[uint16]bool)
    for a:=g.Offsets[src];a<g.Offsets[src+1];a++ {
        if g.Relations[a]!=hdccsr.RelCall {continue}
        u:=g.Edges[a]
        for b:=g.Offsets[u];b<g.Offsets[u+1];b++ {
            if g.Relations[b]!=hdccsr.RelCall {continue}
            w:=g.Edges[b]
            if int(w)!=src {result[w]=true}
        }
    }
    return result
}
func evaluate(eng hdccsr.UnifiedEngine,queries []hdccsr.QueryRequest,truth []map[uint16]bool)metric{
    m:=metric{Name:eng.Name(),Queries:len(queries)}
    latency:=make([]float64,0,len(queries))
    for i,q:=range queries {
        start:=time.Now()
        results:=eng.QueryTopK(q)
        latency=append(latency,float64(time.Since(start).Nanoseconds())/1000)
        hits,first:=0,0
        for j,r:=range results {
            if truth[i][r.NodeID] {
                hits++
                if first==0 {first=j+1}
            }
        }
        m.RecallAt10+=float64(hits)/float64(len(truth[i]))
        m.PrecisionAt10+=float64(hits)/float64(q.K)
        if len(results)!=0 {m.PrecisionOfReturned+=float64(hits)/float64(len(results))}
        if first!=0 {m.MRR+=1.0/float64(first)}
        m.AvgFalsePositives+=float64(len(results)-hits)
    }
    n:=float64(len(queries))
    m.RecallAt10/=n
    m.PrecisionAt10/=n
    m.PrecisionOfReturned/=n
    m.MRR/=n
    m.AvgFalsePositives/=n
    sort.Float64s(latency)
    m.P50US=latency[len(latency)/2]
    m.P95US=latency[int(float64(len(latency)-1)*0.95)]
    for _,v:=range latency {m.MeanUS+=v/n}
    return m
}

func main(){
    manifest:=flag.String("manifest","output/real_ast_manifest.json","Go AST manifest")
    index:=flag.String("index","output/real/knowledge_8k_real_ast.bin","MBVM8K1 v2 index")
    receipt:=flag.String("receipt","output/real/real_export_receipt.json","SHA-256 training receipt")
    out:=flag.String("out","output/real/real_ast_evaluation.json","evaluation JSON")
    limit:=flag.Int("queries",100,"max structural queries")
    maxNodes:=flag.Int("max-symbols",5000,"safety cap for unindexed global scans")
    flag.Parse()
    corpus,err:=hdccsr.LoadRealCorpus(*manifest,*index,*receipt)
    if err!=nil {fmt.Fprintln(os.Stderr,"Invalid real corpus:",err);os.Exit(1)}
    if corpus.Graph.TotalNodes>*maxNodes||*limit<1 {
        fmt.Fprintln(os.Stderr,"Symbol count too large for unindexed benchmark or invalid query limit");os.Exit(1)
    }
    rng:=rand.New(rand.NewSource(42))
    shuffled:=rng.Perm(corpus.Graph.TotalNodes)
    queries:=make([]hdccsr.QueryRequest,0,*limit)
    labels:=make([]map[uint16]bool,0,*limit)
    for _,s:=range shuffled {
        gt:=exactCall2Targets(corpus.Graph,s)
        if len(gt)==0 {continue}
        queries=append(queries,hdccsr.QueryRequest{
            SourceID:uint16(s),Semantic:corpus.Vectors[s],K:10,
        })
        labels=append(labels,gt)
        if len(queries)>=*limit {break}
    }
    if len(queries)==0 {
        fmt.Fprintln(os.Stderr,"No exact RelCall->RelCall 2-hop queries in real AST graph")
        os.Exit(1)
    }
    engineHDC:=hdccsr.NewHDCSystem(42)
    prepStart:=time.Now()
    hop1:=engineHDC.PropagateHop1(corpus.Graph,corpus.Vectors)
    hop2:=engineHDC.PropagateHop2(corpus.Graph,hop1)
    prepMS:=float64(time.Since(prepStart).Nanoseconds())/1e6
    engines:=[]hdccsr.UnifiedEngine{
        hdccsr.NewCSREngine(corpus.Graph,corpus.Vectors),
        hdccsr.NewPureSemanticEngine(corpus.Graph.TotalNodes,corpus.Vectors),
        hdccsr.NewHDCHybridEngine(corpus.Graph.TotalNodes,corpus.Vectors,hop2,0.5),
        hdccsr.NewHDCVSAUnbindEngine(corpus.Graph.TotalNodes,corpus.Vectors,hop2,engineHDC),
    }
    metrics:=make([]metric,0,len(engines))
    for _,engine:=range engines {metrics=append(metrics,evaluate(engine,queries,labels))}
    report:=struct{
        Schema string `json:"schema"`
        Dataset string `json:"dataset"`
        GroundTruth string `json:"ground_truth"`
        IndependentSemanticLabels bool `json:"independent_semantic_labels"`
        SourceDigest string `json:"source_digest"`
        IndexSHA256 string `json:"index_sha256"`
        Symbols int `json:"symbols"`
        Queries int `json:"queries"`
        ContextBuildMS float64 `json:"context_build_ms"`
        Engines []metric `json:"engines"`
    }{"mbvm-real-ast-eval-v1","actual Go declarations / Kaggle-trained projection",
       "exact RelCall->RelCall destinations from experimental CSR (proxy task)",
       false,corpus.Manifest.SourceDigest,corpus.Receipt.IndexSHA256,
       corpus.Graph.TotalNodes,len(queries),prepMS,metrics}
    data,err:=json.MarshalIndent(report,"","  ")
    if err!=nil {fmt.Fprintln(os.Stderr,err);os.Exit(1)}
    if err=os.MkdirAll(filepath.Dir(*out),0755);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
    if err=os.WriteFile(*out,data,0644);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
    fmt.Println(string(data))
}
