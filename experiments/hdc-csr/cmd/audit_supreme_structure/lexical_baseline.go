package main

import (
 "fmt"
 "math"
 "strings"
 "unicode"
)

// Reconstruct exactly the V4 synthetic export text. Source text DOES NOT
// come from the binary index. This baseline requires the V4 provenance;
// it is intentionally not a general classifier for previously unseen Go.
type v4LexicalDomain struct {
 name, title, tags string
}
var v4LexicalDomains = [8]v4LexicalDomain{
 {"concurrency", "Sync & Goroutines", "mutex,atomic,channels,workerpool,deadlock,waitgroup,semaphore,race"},
 {"networking", "High Performance TCP & HTTP", "zerocopy,syncpool,epoll,keepalive,nagle,tls,websocket,multiplex"},
 {"storage", "LSM Tree & B+ Tree Storage", "wal,mmap,compaction,btree,sstable,crc32,iterator,pagecache"},
 {"distributed", "Raft Consensus & Clustering", "raft,election,quorum,heartbeat,logentry,snapshot,replication"},
 {"compiler", "AST Slicing & Code Graph", "ast,traversal,reachability,csr,opcodes,symboltable,cfg,bytecode"},
 {"security", "Cryptographic Signatures & Auth", "ecdsa,sha256,hmac,jwt,bcrypt,aes,entropy,constanttime"},
 {"memory", "Garbage Collection & Allocator", "heap,stack,escapeanalysis,arena,mcache,mspan,zeromemory"},
 {"blast_radius", "Contract Breaking & Mutations", "signature,breakingchange,transitive,mutation,compatibility"},
}

func v4ExportText(index int) string {
 d:=v4LexicalDomains[inferGroup(index)]
 return fmt.Sprintf("%s [Symbol #%d]: Topologia de funções e dependências para %s. Tags: %s.",
  d.title,index,d.name,d.tags)
}

// Represent raw V4 text as a set of lowercase Unicode words, dropping
// pure integer identifiers. This is a conventional bag-of-words/Jaccard
// baseline; domain words remain plainly visible in the input text.
func v4TokenSet(src string) map[string]struct{} {
 tokens:=make(map[string]struct{})
 var word strings.Builder
 add:=func() {
  if word.Len()==0{return}
  token:=word.String();word.Reset()
  numeric:=true
  for _,r:=range token {if !unicode.IsDigit(r){numeric=false;break}}
  if !numeric {tokens[token]=struct{}{}}
 }
 for _,r:=range strings.ToLower(src) {
  if unicode.IsLetter(r)||unicode.IsDigit(r)||r=='_' {
   word.WriteRune(r)
  }else{add()}
 }
 add()
 return tokens
}
func v4Jaccard(a,b map[string]struct{})float64 {
 intersection:=0
 for token:=range a {if _,ok:=b[token];ok{intersection++}}
 union:=len(a)+len(b)-intersection
 if union==0{return 1}
 return float64(intersection)/float64(union)
}
func blockHoldoutQueryID(q,queries int)int{
 block:=q%40
 ordinal:=q/40
 nInBlock:=(queries-1-block)/40+1
 offset:=(ordinal+1)*250/(nInBlock+1)
 return block*250+offset
}
type LexicalHoldoutReport struct {
 Schema string `json:"schema"`
 Queries int `json:"queries"`
 CandidatesPerQuery int `json:"candidates_per_query"`
 Correct int `json:"correct"`
 Top1Accuracy float64 `json:"top1_accuracy"`
 MeanTop1Jaccard float64 `json:"mean_top1_jaccard"`
 RandomSameDomainBaseline float64 `json:"random_same_domain_baseline"`
 HDCMinusLexicalAccuracy float64 `json:"hdc_minus_lexical_accuracy"`
 PerDomainCorrect [8]int `json:"per_domain_correct"`
 Limitations []string `json:"limitations"`
}
func analyzeLexicalBlockHoldout(queries int, hdcAccuracy float64) (LexicalHoldoutReport,error) {
 var r LexicalHoldoutReport
 if queries<1||queries>2000{return r,fmt.Errorf("query count must be 1..2000")}
 if math.IsNaN(hdcAccuracy)||hdcAccuracy<0||hdcAccuracy>1{return r,fmt.Errorf("invalid HDC accuracy")}
 r.Schema="mbvm-v4-lexical-block-holdout-v1"
 r.Queries=queries;r.CandidatesPerQuery=9750
 r.RandomSameDomainBaseline=1000.0/9750.0
 r.Limitations=[]string{
  "Uses reconstructed V4 exporter source strings, not raw text recovered from the binary.",
  "The V4 input explicitly contains domain names and domain keywords; success is lexical/template recognition, not code understanding.",
  "Numeric IDs are removed for Jaccard. Every synthetic domain repeats the same tag vocabulary across source blocks.",
  "Candidate search excludes all 250 items from the query source block, exactly as the HDC audit does.",
  "Assumes the binary index really corresponds to the 10k-item V4 ordering; SHA and version alone cannot prove text provenance.",
 }
 // After removing the numeric ID, all V4 texts belonging to a domain
 // have precisely the same token set. Cache these eight token sets;
 // every candidate still participates in the exact source-block-excluded
 // exhaustive comparison, with tie-breaking by smallest item ID.
 var tokens [8]map[string]struct{}
 for group:=0;group<8;group++ {
  tokens[group]=v4TokenSet(v4ExportText(group*250))
 }
 for q:=0;q<queries;q++ {
  src:=blockHoldoutQueryID(q,queries)
  block:=src/250;group:=inferGroup(src)
  bestScore:=-1.0;bestID:=-1
  for candidate:=0;candidate<10000;candidate++{
   if candidate/250==block{continue}
   score:=v4Jaccard(tokens[group],tokens[inferGroup(candidate)])
   if score>bestScore{
    bestScore,bestID=score,candidate
   }
  }
  if bestID<0{return r,fmt.Errorf("no cross-block lexical candidate")}
  if inferGroup(bestID)==group {r.Correct++;r.PerDomainCorrect[group]++}
  r.MeanTop1Jaccard+=bestScore
 }
 r.MeanTop1Jaccard/=float64(queries)
 r.Top1Accuracy=float64(r.Correct)/float64(queries)
 r.HDCMinusLexicalAccuracy=hdcAccuracy-r.Top1Accuracy
 return r,nil
}
