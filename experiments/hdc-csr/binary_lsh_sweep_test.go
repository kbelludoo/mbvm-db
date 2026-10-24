package hdccsr

import (
	"encoding/json"
	"os"
	"testing"
)

// Sweep is an offline index-selection experiment, not a production tuner.
// It uses fixed synthetic class-prototype vectors and does not claim that
// settings selected here transfer to trained Go-code embeddings.
type lshSweepRow struct {
	Tables int `json:"tables"`
	BitsPerTable int `json:"bits_per_table"`
	ProbeRadius int `json:"probe_radius"`
	Queries int `json:"queries"`
	AverageUniqueCandidates float64 `json:"average_unique_candidates"`
	MaxUniqueCandidates int `json:"max_unique_candidates"`
	AverageCandidateFraction float64 `json:"average_candidate_fraction"`
	OracleTop10Agreement float64 `json:"oracle_top10_agreement"`
	OracleRecallAt10 float64 `json:"oracle_recall_at_10"`
	IndexedRecallAt10 float64 `json:"indexed_recall_at_10"`
	BucketHeaders int `json:"bucket_headers"`
	PostingEntries int `json:"posting_entries"`
}
type lshSweepReport struct {
	Schema string `json:"schema"`
	Source string `json:"source"`
	Nodes int `json:"nodes"`
	Settings []lshSweepRow `json:"settings"`
}

// TestBinaryLSHParameterSweep evaluates candidate pruning and structural
// retrieval quality for 24 combinations. The original exhaustive query is
// computed ONCE per query and used as the shared ranking oracle.
func TestBinaryLSHParameterSweep(t *testing.T) {
	f:=buildMultipathFixture(8080,1000)
	system:=NewHDCSystem(42)
	ctx:=system.PropagateHop2(f.graph,system.PropagateHop1(f.graph,f.vectors))
	oracle:=NewHDCVSAUnbindEngine(len(f.vectors),f.vectors,ctx,system)
	type queryCase struct{
		req QueryRequest
		truth map[uint16]bool
		exact []QueryResult
	}
	cases:=make([]queryCase,0,100)
	for src:=0;src<100;src++ {
		actual:=csrTwoHopCallTargets(f.graph,src)
		if len(actual)==0{continue}
		gt:=make(map[uint16]bool,len(actual))
		for _,id:=range actual {gt[uint16(id)]=true}
		request:=QueryRequest{SourceID:uint16(src),Semantic:f.vectors[src],K:10}
		cases=append(cases,queryCase{request,gt,oracle.QueryTopK(request)})
	}
	if len(cases)==0{t.Fatal("no labeled structural queries")}
	report:=lshSweepReport{
		Schema:"mbvm-binary-lsh-sweep-v1",
		Source:"synthetic 4-class multipath fixture seed 8080; not Kaggle neural embeddings",
		Nodes:len(f.vectors),
		Settings:make([]lshSweepRow,0,24),
	}
	for _,tables:=range []int{4,8,12} {
		for _,bits:=range []int{8,10,12,14} {
			for _,radius:=range []int{0,1} {
				cfg:=BinaryLSHConfig{Tables:tables,BitsPerTable:bits,ProbeRadius:radius,Seed:2026}
				engine,err:=NewHDCVSALSHIndexEngine(f.vectors,ctx,system,cfg)
				if err!=nil{t.Fatal(err)}
				row:=lshSweepRow{
					Tables:tables,BitsPerTable:bits,ProbeRadius:radius,
					Queries:len(cases),
					PostingEntries:engine.Index().BucketEntries(),
				}
				for _,table:=range engine.Index().tables{row.BucketHeaders+=len(table.buckets)}
				for _,c:=range cases {
					// Candidate generation is intentionally measured separately from
					// quality. Query benchmark latency belongs to Go benchmark runs,
					// not to this test's uncontrolled wall clock.
					count:=engine.CandidateCount(c.req)
					if count>len(f.vectors)-1{t.Fatalf("invalid candidate count %d",count)}
					row.AverageUniqueCandidates+=float64(count)
					if count>row.MaxUniqueCandidates{row.MaxUniqueCandidates=count}
					approx:=engine.QueryTopK(c.req)
					oracleSet:=make(map[uint16]bool,len(c.exact))
					for _,v:=range c.exact{oracleSet[v.NodeID]=true}
					oracleHits,indexedHits,matched:=0,0,0
					for _,v:=range c.exact{if c.truth[v.NodeID]{oracleHits++}}
					for _,v:=range approx{
						if c.truth[v.NodeID]{indexedHits++}
						if oracleSet[v.NodeID]{matched++}
					}
					row.OracleRecallAt10+=float64(oracleHits)/float64(len(c.truth))
					row.IndexedRecallAt10+=float64(indexedHits)/float64(len(c.truth))
					row.OracleTop10Agreement+=float64(matched)/float64(len(c.exact))
				}
				q:=float64(len(cases))
				row.AverageUniqueCandidates/=q
				row.AverageCandidateFraction=row.AverageUniqueCandidates/float64(len(f.vectors)-1)
				row.OracleTop10Agreement/=q
				row.OracleRecallAt10/=q
				row.IndexedRecallAt10/=q
				report.Settings=append(report.Settings,row)
				t.Logf("LSH tables=%d bits=%d radius=%d candidate=%.1f%% oracleTop10=%.1f%% structuralRecall=%.3f",
					tables,bits,radius,100*row.AverageCandidateFraction,
					100*row.OracleTop10Agreement,row.IndexedRecallAt10)
			}
		}
	}
	raw,err:=json.MarshalIndent(report,"","  ")
	if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile("lsh_sweep_quality.json",raw,0644);err!=nil{t.Fatal(err)}
}
