package main

import (
 "fmt"
 "math"
 "sort"
)

// BlockHoldoutReport removes every candidate from a query's 250-symbol source
// block. Inferred domain labels are used for scoring, never for choosing
// neighbors. This is ONLY a V4 synthetic-topic diagnostic, not Go semantics.
type BlockHoldoutReport struct {
 Schema string `json:"schema"`
 QueryCount int `json:"query_count"`
 BlockSize int `json:"source_block_size"`
 Blocks int `json:"source_blocks"`
 CandidatesPerQuery int `json:"candidates_per_query"`
 RandomSameDomainBaseline float64 `json:"random_same_domain_baseline"`
 CrossBlockTop1SameDomain int `json:"cross_block_top1_same_domain"`
 CrossBlockTop1Accuracy float64 `json:"cross_block_top1_accuracy"`
 CrossBlockNonidenticalTop1SameDomain int `json:"cross_block_nonidentical_top1_same_domain"`
 CrossBlockNonidenticalTop1Accuracy float64 `json:"cross_block_nonidentical_top1_accuracy"`
 ExactDuplicateCrossBlockNeighbors int `json:"exact_duplicate_cross_block_neighbors"`
 MeanClosestSameDomainBits float64 `json:"mean_closest_same_domain_bits"`
 MeanClosestOtherDomainBits float64 `json:"mean_closest_other_domain_bits"`
 MeanSeparationMarginBits float64 `json:"mean_other_minus_same_bits"`
 MedianSeparationMarginBits float64 `json:"median_other_minus_same_bits"`
 MinimumSeparationMarginBits int `json:"minimum_other_minus_same_bits"`
 PerDomainQueries [8]int `json:"per_domain_queries"`
 PerDomainCorrect [8]int `json:"per_domain_correct"`
 Caveats []string `json:"caveats"`
}

func analyzeBlockHoldout(vectors [][]byte, queries int) (BlockHoldoutReport,error) {
 const blockSize=250
 const totalBlocks=40
 var r BlockHoldoutReport
 r.Schema="mbvm-synthetic-v4-block-holdout-v1"
 if len(vectors)!=blockSize*totalBlocks {return r,fmt.Errorf("V4 layout requires 10000 vectors, got %d",len(vectors))}
 if queries<1||queries>2000{return r,fmt.Errorf("block-holdout queries must be 1..2000")}
 r.QueryCount=queries
 r.BlockSize=blockSize
 r.Blocks=totalBlocks
 r.CandidatesPerQuery=len(vectors)-blockSize
 // Five 250-symbol blocks per domain; one whole source block excluded.
 r.RandomSameDomainBaseline=float64((5-1)*blockSize)/float64(r.CandidatesPerQuery)
 r.Caveats=[]string{
  "Labels depend on an assumed V4 synthetic generator and output row ordering, not actual source-code behavior.",
  "Generated input text explicitly contains domain names and tags. A lexical baseline can classify these templates without training.",
  "Cross-block retrieval still reuses domain vocabulary and the same input template; it is not OOD code generalization.",
  "Cross-block exact duplicate neighbors are reported; nonidentical Top1 also excludes zero-distance candidates.",
  "Random candidate baseline assumes uniform sampling of non-source-block records and is not a test-set p-value.",
 }
 margins:=make([]int,0,queries)
 var sameTotal,otherTotal int64
 for q:=0;q<queries;q++{
  src:=blockHoldoutQueryID(q,queries)
  block:=src/blockSize
  label:=inferGroup(src)
  r.PerDomainQueries[label]++
  bestOverallDist,bestOverallID:=vectorBits+1,-1
  bestDistinctDist,bestDistinctID:=vectorBits+1,-1
  bestSameDist,bestOtherDist:=vectorBits+1,vectorBits+1
  for id,candidate:=range vectors{
   if id/blockSize==block{continue}
   d:=hamming(vectors[src],candidate)
   if d<bestOverallDist{bestOverallDist,bestOverallID=d,id}
   if d>0&&d<bestDistinctDist{bestDistinctDist,bestDistinctID=d,id}
   if inferGroup(id)==label{
    if d<bestSameDist{bestSameDist=d}
   }else if d<bestOtherDist{bestOtherDist=d}
  }
  if bestOverallID<0||bestDistinctID<0||bestSameDist>vectorBits||bestOtherDist>vectorBits{
   return r,fmt.Errorf("missing candidates for query ID %d",src)
  }
  if inferGroup(bestOverallID)==label{r.CrossBlockTop1SameDomain++;r.PerDomainCorrect[label]++}
  if inferGroup(bestDistinctID)==label{r.CrossBlockNonidenticalTop1SameDomain++}
  if bestOverallDist==0{r.ExactDuplicateCrossBlockNeighbors++}
  sameTotal+=int64(bestSameDist)
  otherTotal+=int64(bestOtherDist)
  margins=append(margins,bestOtherDist-bestSameDist)
 }
 sort.Ints(margins)
 r.CrossBlockTop1Accuracy=float64(r.CrossBlockTop1SameDomain)/float64(queries)
 r.CrossBlockNonidenticalTop1Accuracy=float64(r.CrossBlockNonidenticalTop1SameDomain)/float64(queries)
 r.MeanClosestSameDomainBits=float64(sameTotal)/float64(queries)
 r.MeanClosestOtherDomainBits=float64(otherTotal)/float64(queries)
 r.MeanSeparationMarginBits=float64(otherTotal-sameTotal)/float64(queries)
 r.MinimumSeparationMarginBits=margins[0]
 if queries%2!=0{r.MedianSeparationMarginBits=float64(margins[queries/2])}else{
  r.MedianSeparationMarginBits=float64(margins[queries/2-1]+margins[queries/2])/2
 }
 if math.IsNaN(r.MeanSeparationMarginBits){return r,fmt.Errorf("invalid margin")}
 return r,nil
}
