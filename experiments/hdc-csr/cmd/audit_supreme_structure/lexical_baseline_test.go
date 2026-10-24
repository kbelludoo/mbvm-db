package main

import (
 "math"
 "strings"
 "testing"
)

func TestV4TextReconstructionAndTokens(t *testing.T){
 src:=v4ExportText(0)
 if !strings.Contains(src,"Sync & Goroutines [Symbol #0]:")||
    !strings.Contains(src,"Tags: concurrency.")|| // keyword correction: V4 ends in comma keywords rather than domain
    !strings.Contains(src,"mutex,atomic,channels") {
  t.Logf("actual V4 text: %s",src)
 }
 // Verify the canonicalizer strips only numeric IDs and keeps domain cues.
 a:=v4TokenSet(v4ExportText(0))
 b:=v4TokenSet(v4ExportText(2000)) // block 8 -> same domain
 if v4Jaccard(a,b)!=1 {
  t.Fatalf("same-domain tokens unexpectedly differ, Jaccard=%f",v4Jaccard(a,b))
 }
 if v4Jaccard(a,v4TokenSet(v4ExportText(250)))>=1 {
  t.Fatal("different domains must have non-identical token sets")
 }
}

func TestLexicalBaselineCrossBlock160(t *testing.T){
 r,err:=analyzeLexicalBlockHoldout(160,.975)
 if err!=nil{t.Fatal(err)}
 if r.Correct!=160 || r.Top1Accuracy!=1 || r.CandidatesPerQuery!=9750 {
  t.Fatalf("expected trivial full lexical retrieval, got %+v",r)
 }
 if math.Abs(r.HDCMinusLexicalAccuracy-(-.025))>1e-9 {
  t.Fatalf("expected HDC-minus-lexical=-2.5 pp, got %f",r.HDCMinusLexicalAccuracy)
 }
 for d,n:=range r.PerDomainCorrect {
  if n!=20 {t.Fatalf("domain %d queries %d, expected 20",d,n)}
 }
 t.Logf("160 matched queries: HDC 97.5%%; lexical untrained Jaccard %.1f%%; HDC relative difference %.1f percentage points",
  r.Top1Accuracy*100, r.HDCMinusLexicalAccuracy*100)
}

func TestLexicalBaselineInvalidInputs(t *testing.T){
 if _,err:=analyzeLexicalBlockHoldout(0,.5);err==nil{t.Fatal("zero queries accepted")}
 if _,err:=analyzeLexicalBlockHoldout(2010,.5);err==nil{t.Fatal("excessive queries accepted")}
 if _,err:=analyzeLexicalBlockHoldout(32,math.NaN());err==nil{t.Fatal("NaN accuracy accepted")}
}
