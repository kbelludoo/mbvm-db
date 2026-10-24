package main

import (
 "crypto/sha256"
 "encoding/binary"
 "encoding/hex"
 "encoding/json"
 "flag"
 "fmt"
 "math"
 "math/bits"
 "math/rand"
 "os"
 "sort"
 "strings"
)

const ( vectorBits=8192; vectorBytes=1024 )

type HammingSample struct {
 Count int `json:"count"`
 Mean float64 `json:"mean"`
 P50 int `json:"p50"`
 P90 int `json:"p90"`
}
type Result struct {
 Schema string `json:"schema"`
 FileSHA256 string `json:"file_sha256"`
 Version uint32 `json:"version"`
 Vectors int `json:"vectors"`
 RandomSeed int64 `json:"random_seed"`
 SampledColumnPairs int `json:"sampled_column_pairs"`
 MeanAbsBitPhi float64 `json:"mean_absolute_interbit_phi"`
 P95AbsBitPhi float64 `json:"p95_absolute_interbit_phi"`
 MaximumAbsBitPhi float64 `json:"maximum_absolute_interbit_phi"`
 IndependentAbsPhiReference float64 `json:"independent_abs_phi_reference_approx"`
 Layout string `json:"layout"`
 DomainNames []string `json:"domain_names,omitempty"`
 GroupWithin *HammingSample `json:"within_inferred_domain,omitempty"`
 GroupAcross *HammingSample `json:"across_inferred_domain,omitempty"`
 MeanDistanceGap *float64 `json:"across_minus_within_hamming,omitempty"`
 NeighborQueries int `json:"nearest_neighbor_queries,omitempty"`
 NearestSameGroupRate *float64 `json:"nearest_same_inferred_domain_rate,omitempty"`
 RandomSameGroupBaseline *float64 `json:"random_same_inferred_domain_baseline,omitempty"`
 BlockHoldout *BlockHoldoutReport `json:"block_holdout,omitempty"`
 Caveats []string `json:"caveats"`
}
var domains=[]string{"concurrency","networking","storage","distributed","compiler","security","memory","blast_radius"}

func hamming(a,b []byte) int {
 n:=0
 for w:=0;w<vectorBytes;w+=8 { n+=bits.OnesCount64(binary.LittleEndian.Uint64(a[w:])^binary.LittleEndian.Uint64(b[w:])) }
 return n
}
func load(raw []byte,maxVectors int)([][]byte,uint32,error) {
 if len(raw)<18{return nil,0,fmt.Errorf("truncated header")}
 if string(raw[:8])!="MBVM8K1\x00"{return nil,0,fmt.Errorf("bad magic")}
 ver:=binary.LittleEndian.Uint32(raw[8:12]);header:=18
 if ver==2{header=64}else if ver!=1{return nil,0,fmt.Errorf("unsupported version %d",ver)}
 n:=int(binary.LittleEndian.Uint32(raw[12:16]))
 if n<2||n>maxVectors||binary.LittleEndian.Uint16(raw[16:18])!=vectorBits{return nil,0,fmt.Errorf("invalid count or dimensions")}
 if len(raw)!=header+n*vectorBytes{return nil,0,fmt.Errorf("file length mismatch: got %d want %d",len(raw),header+n*vectorBytes)}
 if ver==2{for _,b:=range raw[18:64]{if b!=0{return nil,0,fmt.Errorf("nonzero padding")}}}
 v:=make([][]byte,n)
 for i:=range v{start:=header+i*vectorBytes;v[i]=raw[start:start+vectorBytes]}
 return v,ver,nil
}
func bitAt(v []byte,pos int)bool{return v[pos/8]&(1<<uint(pos%8))!=0}
func inferGroup(i int)int{return (i/250)%8}
func sampleStat(v []int)HammingSample {
 r:=HammingSample{Count:len(v)}
 if len(v)==0{return r}
 a:=append([]int(nil),v...);sort.Ints(a);sum:=0
 for _,d:=range a{sum+=d}
 r.Mean=float64(sum)/float64(len(a));r.P50=a[len(a)/2];r.P90=a[int(float64(len(a)-1)*.90)]
 return r
}
func analyze(vectors [][]byte,version uint32,seed int64,columnPairs,domainPairs,nearestQueries int,withDomains bool)Result{
 rng:=rand.New(rand.NewSource(seed));n:=len(vectors)
 r:=Result{Schema:"mbvm-structure-audit-v1",Version:version,Vectors:n,RandomSeed:seed,Layout:"none",
  Caveats:[]string{
   "Interbit Pearson phi identifies sampled positional association, not semantics or causation.",
   "Independent-phi reference assumes independent nondegenerate Bernoulli columns and is approximate, not a significance test.",
   "Domain labels are inferred only from V4 synthetic generator chunk order, not real Go AST behavior.",
   "Neither source-code semantics nor the original trained projection weights can be recovered from this index.",
  }}
 correlations:=make([]float64,0,columnPairs)
 for k:=0;k<columnPairs;k++{
  a:=rng.Intn(vectorBits);b:=rng.Intn(vectorBits-1);if b>=a{b++}
  na,nb,nab:=0,0,0
  for _,v:=range vectors{
   x,y:=bitAt(v,a),bitAt(v,b)
   if x{na++};if y{nb++};if x&&y{nab++}
  }
  pa,pb,pab:=float64(na)/float64(n),float64(nb)/float64(n),float64(nab)/float64(n)
  den:=math.Sqrt(pa*(1-pa)*pb*(1-pb))
  if den>0{correlations=append(correlations,math.Abs((pab-pa*pb)/den))}
 }
 sort.Float64s(correlations);r.SampledColumnPairs=len(correlations)
 if len(correlations)>0 {
  for _,c:=range correlations{r.MeanAbsBitPhi+=c}
  r.MeanAbsBitPhi/=float64(len(correlations))
  r.P95AbsBitPhi=correlations[int(float64(len(correlations)-1)*.95)]
  r.MaximumAbsBitPhi=correlations[len(correlations)-1]
 }
 r.IndependentAbsPhiReference=math.Sqrt(2/(math.Pi*float64(n-1)))
 if !withDomains{return r}
 r.Layout="synthetic-v4-assumed: (index/250)%8";r.DomainNames=append([]string(nil),domains...)
 within,across:=make([]int,0,domainPairs),make([]int,0,domainPairs)
 for len(within)<domainPairs||len(across)<domainPairs {
  i:=rng.Intn(n);j:=rng.Intn(n-1);if j>=i{j++}
  if inferGroup(i)==inferGroup(j) {
   if len(within)<domainPairs{within=append(within,hamming(vectors[i],vectors[j]))}
  } else if len(across)<domainPairs {
   across=append(across,hamming(vectors[i],vectors[j]))
  }
 }
 w,a:=sampleStat(within),sampleStat(across);gap:=a.Mean-w.Mean
 r.GroupWithin=&w;r.GroupAcross=&a;r.MeanDistanceGap=&gap
 if nearestQueries>n{nearestQueries=n}
 r.NeighborQueries=nearestQueries;same:=0
 for q:=0;q<nearestQueries;q++{
  id:=(q*n)/nearestQueries;best,bestDist:=-1,vectorBits+1
  for i,vec:=range vectors {
   if i==id{continue}
   d:=hamming(vectors[id],vec)
   if d<bestDist{best,bestDist=i,d}
  }
  if inferGroup(best)==inferGroup(id){same++}
 }
 nearRate:=float64(same)/float64(nearestQueries);r.NearestSameGroupRate=&nearRate
 var counts [8]int
 for i:=0;i<n;i++{counts[inferGroup(i)]++}
 count:=0;for _,c:=range counts{count+=c*(c-1)}
 baseline:=float64(count)/float64(n*(n-1));r.RandomSameGroupBaseline=&baseline
 return r
}
func main(){
 file:=flag.String("file","output/knowledge_8k_supreme.bin","binary index")
 expected:=flag.String("expected-sha256","","expected SHA256 of trusted index")
 seed:=flag.Int64("seed",2026,"reproducible seed")
 columns:=flag.Int("column-pairs",512,"sampled pairs of bit positions")
 domainPairs:=flag.Int("domain-pairs",2048,"pairs in each inferred-domain stratum")
 queries:=flag.Int("nearest-queries",32,"fullscan neighbor queries")
 holdoutQueries:=flag.Int("block-holdout-queries",160,"V4-only queries excluding all candidates from the same 250-record block; set 0 to disable")
 synthetic:=flag.Bool("synthetic-v4-layout",false,"enable label inference ONLY for the V4 250-row/8-domain synthetic trainer")
 limit:=flag.Int("max-vectors",100000,"vector-count safety limit")
 out:=flag.String("out","","optional output JSON")
 flag.Parse()
 if *columns<1||*domainPairs<1||*queries<1||*holdoutQueries<0||*limit<2{fmt.Fprintln(os.Stderr,"sample counts must be positive");os.Exit(2)}
 stat,err:=os.Stat(*file);if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 if stat.Size()<18||stat.Size()>64+int64(*limit)*vectorBytes{fmt.Fprintln(os.Stderr,"file size outside limit");os.Exit(1)}
 raw,err:=os.ReadFile(*file);if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 hash:=sha256.Sum256(raw);sha:=hex.EncodeToString(hash[:])
 if *expected!=""&&!strings.EqualFold(*expected,sha){fmt.Fprintln(os.Stderr,"SHA mismatch:",sha);os.Exit(1)}
 vectors,ver,err:=load(raw,*limit);if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 if *synthetic&&len(vectors)!=10000{fmt.Fprintln(os.Stderr,"V4 synthetic label inference requires 10000 vectors");os.Exit(1)}
 result:=analyze(vectors,ver,*seed,*columns,*domainPairs,*queries,*synthetic)
 if *synthetic&&*holdoutQueries>0 {
  holdout,err:=analyzeBlockHoldout(vectors,*holdoutQueries)
  if err!=nil{fmt.Fprintln(os.Stderr,"V4 block holdout:",err);os.Exit(1)}
  result.BlockHoldout=&holdout
 }
 result.FileSHA256=sha
 b,err:=json.MarshalIndent(result,"","  ");if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)};b=append(b,'\n')
 if *out!=""{if err=os.WriteFile(*out,b,0600);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
 fmt.Print(string(b))
}
