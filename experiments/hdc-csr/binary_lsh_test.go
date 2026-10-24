package hdccsr

import (
	"encoding/json"
	"math/rand"
	"os"
	"testing"
)

func TestBinaryLSHExactIdenticalVectorRetrieval(t *testing.T) {
	rng:=rand.New(rand.NewSource(2026))
	vec:=make([]Hypervector,64)
	for i:=range vec { vec[i]=GenerateRandomHypervector(rng) }
	// A duplicate vector is guaranteed to collide across every table.
	vec[33]=vec[7]
	for _,radius:=range []int{0,1,2} {
		idx,err:=NewBinaryLSHIndex(vec,BinaryLSHConfig{
			Tables:8,BitsPerTable:10,ProbeRadius:radius,Seed:123,
		})
		if err!=nil {t.Fatal(err)}
		ids:=idx.CandidateIDs(vec[7],7)
		found:=false
		for _,id:=range ids {
			if id==7 {t.Fatal("source included in candidates")}
			if id==33 {found=true}
		}
		if !found {t.Fatalf("radius %d lost an exact duplicate",radius)}
		if idx.BucketEntries()!=8*len(vec) {t.Fatalf("wrong bucket entries: %d",idx.BucketEntries())}
	}
}

func TestBinaryLSHConfiguration(t *testing.T){
	orig:=make([]Hypervector,10)
	for _,cfg:=range []BinaryLSHConfig{
		{Tables:0,BitsPerTable:10,ProbeRadius:1},
		{Tables:10,BitsPerTable:0,ProbeRadius:1},
		{Tables:10,BitsPerTable:17,ProbeRadius:1},
		{Tables:10,BitsPerTable:10,ProbeRadius:-1},
		{Tables:10,BitsPerTable:10,ProbeRadius:3},
		{Tables:600,BitsPerTable:16,ProbeRadius:1},
	}{
		if _,err:=NewBinaryLSHIndex(orig,cfg);err==nil{
			t.Fatalf("invalid LSH config accepted: %+v",cfg)
		}
	}
}

func TestBinaryLSHScoreMatchesExactRescoring(t *testing.T) {
	f:=buildBenchmarkEnvironmentForLSH()
	sys:=NewHDCSystem(42)
	hdc:=sys.PropagateHop2(f.graph,sys.PropagateHop1(f.graph,f.vectors))
	oracle:=NewHDCVSAUnbindEngine(len(f.vectors),f.vectors,hdc,sys)
	approx,err:=NewHDCVSALSHIndexEngine(f.vectors,hdc,sys,BinaryLSHConfig{
		Tables:8,BitsPerTable:6,ProbeRadius:2,Seed:2026,
	})
	if err!=nil {t.Fatal(err)}
	for src:=0;src<10;src++ {
		req:=QueryRequest{SourceID:uint16(src),Semantic:f.vectors[src],K:10}
		exact:=oracle.QueryTopK(req)
		ls:=approx.QueryTopK(req)
		if len(ls)>10 {t.Fatalf("returned > K")}
		for _,x:=range ls {
			found:=false
			for _,y:=range exact {
				if x.NodeID==y.NodeID && x.Score==y.Score {found=true;break}
			}
			// More candidates may tie on score; only exact score equivalence
			// is universally required, not an identical tie order.
			if !found {
				expected:=ScoreSemanticOnly(req.Semantic,f.vectors[x.NodeID])
				cfg:=sys.RelConfigs[RelCall]
				role:=Bind(RotL8K(cfg.RoleVector,cfg.Shift),cfg.RoleVector)
				pred:=RotR8K(Unbind(hdc.Contextual[src],role),cfg.Shift*2)
				if hdc.HasContext[src] {
					other:=ScoreSemanticOnly(pred,f.vectors[x.NodeID])
					if other>expected {expected=other}
				}
				if expected!=x.Score {t.Fatalf("LSH rescoring differs from full formula")}
			}
		}
	}
}

type lshFixture struct{
	graph *ExpCSRGraph
	vectors []Hypervector
}
func buildBenchmarkEnvironmentForLSH()lshFixture{
	f:=buildMultipathFixture(8080,1000)
	return lshFixture{f.graph,f.vectors}
}

type lshQualityReport struct{
	Schema string `json:"schema"`
	Nodes int `json:"nodes"`
	Queries int `json:"queries"`
	Tables int `json:"tables"`
	BitsPerTable int `json:"bits_per_table"`
	ProbeRadius int `json:"probe_radius"`
	OracleRecallAt10 float64 `json:"oracle_recall_at_10"`
	LSHRecallAt10 float64 `json:"lsh_recall_at_10"`
	AvgOracleTop10Agreement float64 `json:"avg_oracle_top10_agreement"`
	AvgUniqueCandidates float64 `json:"avg_unique_candidates"`
	MaxUniqueCandidates int `json:"max_unique_candidates"`
	IndexPostingEntries int `json:"index_posting_entries"`
	Evaluation string `json:"evaluation"`
}

func TestBinaryLSHQualityE2E(t *testing.T) {
	f:=buildBenchmarkEnvironmentForLSH()
	sys:=NewHDCSystem(42)
	ctx:=sys.PropagateHop2(f.graph,sys.PropagateHop1(f.graph,f.vectors))
	oracle:=NewHDCVSAUnbindEngine(len(f.vectors),f.vectors,ctx,sys)
	cfg:=BinaryLSHConfig{Tables:12,BitsPerTable:10,ProbeRadius:1,Seed:2026}
	approx,err:=NewHDCVSALSHIndexEngine(f.vectors,ctx,sys,cfg)
	if err!=nil {t.Fatal(err)}
	report:=lshQualityReport{
		Schema:"mbvm-binary-lsh-v1",Nodes:len(f.vectors),Queries:100,
		Tables:cfg.Tables,BitsPerTable:cfg.BitsPerTable,
		ProbeRadius:cfg.ProbeRadius,IndexPostingEntries:approx.Index().BucketEntries(),
		Evaluation:"synthetic_accuracy_and_candidate_pruning_not_neural_generalization",
	}
	queries:=0
	for src:=0;src<100;src++ {
		req:=QueryRequest{SourceID:uint16(src),Semantic:f.vectors[src],K:10}
		gtIDs:=csrTwoHopCallTargets(f.graph,src)
		if len(gtIDs)==0 {continue}
		gt:=make(map[uint16]bool,len(gtIDs))
		for _,id:=range gtIDs {gt[uint16(id)]=true}
		baseline:=oracle.QueryTopK(req)
		indexed:=approx.QueryTopK(req)
		oracleTop:=make(map[uint16]bool,len(baseline))
		for _,x:=range baseline {oracleTop[x.NodeID]=true}
		oracleHits,indexedHits,agreement:=0,0,0
		for _,x:=range baseline {if gt[x.NodeID]{oracleHits++}}
		for _,x:=range indexed {
			if gt[x.NodeID]{indexedHits++}
			if oracleTop[x.NodeID]{agreement++}
		}
		report.OracleRecallAt10+=float64(oracleHits)/float64(len(gt))
		report.LSHRecallAt10+=float64(indexedHits)/float64(len(gt))
		report.AvgOracleTop10Agreement+=float64(agreement)/float64(len(baseline))
		num:=approx.CandidateCount(req)
		report.AvgUniqueCandidates+=float64(num)
		if num>report.MaxUniqueCandidates {report.MaxUniqueCandidates=num}
		if num>len(f.vectors)-1 {
			t.Fatal("candidate accounting exceeded valid IDs")
		}
		queries++
	}
	if queries==0 {t.Fatal("no structural queries")}
	report.Queries=queries
	report.OracleRecallAt10/=float64(queries)
	report.LSHRecallAt10/=float64(queries)
	report.AvgOracleTop10Agreement/=float64(queries)
	report.AvgUniqueCandidates/=float64(queries)
	t.Logf("LSH mean candidates: %.1f/%d; oracle recall@10 %.3f; LSH recall@10 %.3f; oracle top10 agreement %.3f",
		report.AvgUniqueCandidates,len(f.vectors),
		report.OracleRecallAt10,report.LSHRecallAt10,report.AvgOracleTop10Agreement)
	out,err:=json.MarshalIndent(report,"","  ")
	if err!=nil {t.Fatal(err)}
	if err:=os.WriteFile("lsh_quality.json",out,0644);err!=nil {t.Fatal(err)}
}

var binaryLSHBenchmarkSink []QueryResult

func BenchmarkVSA_FullScan_1000nodes_Matched(b *testing.B){
	f:=buildBenchmarkEnvironmentForLSH()
	sys:=NewHDCSystem(42)
	ctx:=sys.PropagateHop2(f.graph,sys.PropagateHop1(f.graph,f.vectors))
	eng:=NewHDCVSAUnbindEngine(len(f.vectors),f.vectors,ctx,sys)
	b.ReportAllocs()
	b.ResetTimer()
	for i:=0;i<b.N;i++{
		s:=uint16(i%100)
		binaryLSHBenchmarkSink=eng.QueryTopK(QueryRequest{SourceID:s,Semantic:f.vectors[s],K:10})
	}
}
func BenchmarkVSA_LSH_1000nodes_Matched(b *testing.B){
	f:=buildBenchmarkEnvironmentForLSH()
	sys:=NewHDCSystem(42)
	ctx:=sys.PropagateHop2(f.graph,sys.PropagateHop1(f.graph,f.vectors))
	eng,err:=NewHDCVSALSHIndexEngine(f.vectors,ctx,sys,BinaryLSHConfig{
		Tables:12,BitsPerTable:10,ProbeRadius:1,Seed:2026,
	})
	if err!=nil {b.Fatal(err)}
	b.ReportAllocs()
	b.ResetTimer()
	for i:=0;i<b.N;i++{
		s:=uint16(i%100)
		binaryLSHBenchmarkSink=eng.QueryTopK(QueryRequest{SourceID:s,Semantic:f.vectors[s],K:10})
	}
}
