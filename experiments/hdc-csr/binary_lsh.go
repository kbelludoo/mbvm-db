package hdccsr

import (
	"fmt"
	"math/rand"
	"sort"
)

// BinaryLSHConfig controls approximate candidate generation over 8192-bit
// ORIGINAL vectors. Query results are exact rescored only among retrieved IDs.
type BinaryLSHConfig struct {
	Tables      int
	BitsPerTable int
	ProbeRadius int // 0, 1 or 2
	Seed        int64
}

type binaryLSHTable struct {
	bits []uint16
	buckets [][]uint16
}

// BinaryLSHIndex uses independent, seeded bit samples with multiprobe
// buckets. It is approximate; querying both semantic and VSA-predicted
// vectors is necessary for the max(semantic,VSA) baseline.
type BinaryLSHIndex struct {
	config BinaryLSHConfig
	tables []binaryLSHTable
	count int
	bucketEntries int
}

func NewBinaryLSHIndex(original []Hypervector, cfg BinaryLSHConfig) (*BinaryLSHIndex,error) {
	if len(original)==0 || len(original)>65536 {
		return nil,fmt.Errorf("BinaryLSH requires 1..65536 vectors (uint16 ids); got %d",len(original))
	}
	if cfg.Tables<1 || cfg.BitsPerTable<1 || cfg.BitsPerTable>16 ||
		cfg.Tables*cfg.BitsPerTable>VectorBits ||
		cfg.ProbeRadius<0 || cfg.ProbeRadius>2 {
		return nil,fmt.Errorf("invalid BinaryLSH config: %+v",cfg)
	}
	index:=&BinaryLSHIndex{config:cfg,count:len(original)}
	positions:=rand.New(rand.NewSource(cfg.Seed)).Perm(VectorBits)
	index.tables=make([]binaryLSHTable,cfg.Tables)
	for t:=0;t<cfg.Tables;t++ {
		bits:=make([]uint16,cfg.BitsPerTable)
		for j:=range bits { bits[j]=uint16(positions[t*cfg.BitsPerTable+j]) }
		tab:=binaryLSHTable{bits:bits,buckets:make([][]uint16,1<<uint(cfg.BitsPerTable))}
		for id,v:=range original {
			sig:=binaryLSHSignature(v,bits)
			tab.buckets[sig]=append(tab.buckets[sig],uint16(id))
			index.bucketEntries++
		}
		index.tables[t]=tab
	}
	return index,nil
}

func binaryLSHSignature(v Hypervector,bits []uint16) uint16 {
	var sig uint16
	for j,bit:=range bits {
		word:=bit/64
		offset:=bit%64
		sig|=uint16((v[word]>>offset)&1)<<uint(j)
	}
	return sig
}

// CandidateIDs probes each table up to radius 2 (Hamming distance in the
// sampled signature), deduplicates by ID, and excludes source. The work is
// proportional to probed buckets and visited postings, plus a small map.
func (idx *BinaryLSHIndex) CandidateIDs(query Hypervector, source uint16) []uint16 {
	if idx==nil{return nil}
	seen:=make(map[uint16]struct{},16)
	add:=func(bucket []uint16){
		for _,id:=range bucket {
			if id!=source {seen[id]=struct{}{}}
		}
	}
	for _,tab:=range idx.tables {
		key:=binaryLSHSignature(query,tab.bits)
		add(tab.buckets[key])
		if idx.config.ProbeRadius>=1 {
			for i:=0;i<len(tab.bits);i++ {
				add(tab.buckets[key^(1<<uint(i))])
			}
		}
		if idx.config.ProbeRadius>=2 {
			for i:=0;i<len(tab.bits);i++ {
				for j:=i+1;j<len(tab.bits);j++ {
					add(tab.buckets[key^(1<<uint(i))^(1<<uint(j))])
				}
			}
		}
	}
	ids:=make([]uint16,0,len(seen))
	for id:=range seen {ids=append(ids,id)}
	return ids
}

func (idx *BinaryLSHIndex) BucketEntries() int { if idx==nil{return 0};return idx.bucketEntries }
func (idx *BinaryLSHIndex) TableCount() int { if idx==nil{return 0};return len(idx.tables) }

// HDCVSALSHIndexEngine applies the SAME ranking score as
// HDCVSAUnbindEngine (max of semantic and unbound-VSA similarity), but
// considers only IDs proposed by the BinaryLSH index.
// Recall is NOT guaranteed; test against an exhaustive oracle.
type HDCVSALSHIndexEngine struct {
	original []Hypervector
	mem *ContextualMemory
	role Hypervector
	shift int
	index *BinaryLSHIndex
}

func NewHDCVSALSHIndexEngine(orig []Hypervector,mem *ContextualMemory,sys *HDCSystem,cfg BinaryLSHConfig)(*HDCVSALSHIndexEngine,error) {
	idx,err:=NewBinaryLSHIndex(orig,cfg)
	if err!=nil{return nil,err}
	call:=sys.RelConfigs[RelCall]
	role:=Bind(RotL8K(call.RoleVector,call.Shift),call.RoleVector)
	return &HDCVSALSHIndexEngine{original:orig,mem:mem,role:role,shift:call.Shift*2,index:idx},nil
}
func(e *HDCVSALSHIndexEngine) Name()string{return "HDC_Canonical_VSA_LSH"}
func(e *HDCVSALSHIndexEngine) Index() *BinaryLSHIndex {return e.index}

// candidateIDs uses two separate hash queries because a max() score can
// be high for either query. If no HDC context exists, only semantic is used.
func(e *HDCVSALSHIndexEngine) candidateIDs(req QueryRequest)([]uint16,Hypervector,bool){
	has:=e.mem.HasContext[req.SourceID]
	var predicted Hypervector
	if has {
		predicted=RotR8K(Unbind(e.mem.Contextual[req.SourceID],e.role),e.shift)
	}
	seen:=make(map[uint16]struct{})
	for _,id:=range e.index.CandidateIDs(req.Semantic,req.SourceID){seen[id]=struct{}{}}
	if has {
		for _,id:=range e.index.CandidateIDs(predicted,req.SourceID){seen[id]=struct{}{}}
	}
	ids:=make([]uint16,0,len(seen))
	for id:=range seen{ids=append(ids,id)}
	return ids,predicted,has
}

func(e *HDCVSALSHIndexEngine) CandidateCount(req QueryRequest)int{
	ids,_,_:=e.candidateIDs(req)
	return len(ids)
}
func(e *HDCVSALSHIndexEngine) QueryTopK(req QueryRequest)[]QueryResult{
	if req.K<=0{return nil}
	ids,predicted,has:=e.candidateIDs(req)
	candidates:=make([]QueryResult,0,len(ids))
	for _,id:=range ids {
		score:=ScoreSemanticOnly(req.Semantic,e.original[id])
		if has {
			other:=ScoreSemanticOnly(predicted,e.original[id])
			if other>score {score=other}
		}
		candidates=append(candidates,QueryResult{NodeID:id,Score:score})
	}
	sort.Slice(candidates,func(i,j int)bool {
		if candidates[i].Score==candidates[j].Score {
			return candidates[i].NodeID<candidates[j].NodeID
		}
		return candidates[i].Score>candidates[j].Score
	})
	if len(candidates)>req.K{candidates=candidates[:req.K]}
	return candidates
}
