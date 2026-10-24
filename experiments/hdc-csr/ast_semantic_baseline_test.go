package hdccsr

import (
 "math"
 "strings"
 "testing"
)

func TestASTBaselineFunctionalSensitivityControls(t *testing.T) {
 cases:=readSemanticSensitivityFixtures(t)
 certs,witnesses:=0,0
 inputs:=[]int{-100,-30,-11,-10,-7,-2,-1,0,1,2,7,10,11,30,100}
 for _,c:=range cases.Cases {
  proven,err:=ASTConservativeEquivalent(c.Original,c.Equivalent)
  if err!=nil{t.Fatalf("%s equivalent parse: %v",c.ID,err)}
  if !proven{t.Errorf("%s: expected canonical algebraic equivalence",c.ID)}
  if proven{certs++}
  changedProven,err:=ASTConservativeEquivalent(c.Original,c.Changed)
  if err!=nil{t.Fatalf("%s changed parse: %v",c.ID,err)}
  if changedProven{t.Errorf("%s: mutation falsely certified equivalent",c.ID)}
  a,err:=parseArithmeticFunction(c.Original)
  if err!=nil{t.Fatal(err)}
  b,err:=parseArithmeticFunction(c.Changed)
  if err!=nil{t.Fatal(err)}
  differenceSeen:=false
  for _,x:=range inputs {
   for _,y:=range inputs {
    av,err:=evalArithmeticFunction(a,x,y)
    if err!=nil{t.Fatal(err)}
    bv,err:=evalArithmeticFunction(b,x,y)
    if err!=nil{t.Fatal(err)}
    if av!=bv{differenceSeen=true}
   }
  }
  if differenceSeen{witnesses++}else{t.Errorf("%s: mutation lacked a behavioral witness",c.ID)}
  t.Logf("%s: equivalent certified=%t, changed certified=%t, behavioral change witnessed=%t",c.ID,proven,changedProven,differenceSeen)
 }
 if certs!=len(cases.Cases)||witnesses!=len(cases.Cases){
  t.Fatalf("expected all five certificates and mutation witnesses; got %d and %d",certs,witnesses)
 }
}

// Unsupported Go semantics MUST NOT be silently equated using a weak normalizer.
func TestASTBaselineUnsupportedSafetyAndCommutativity(t *testing.T){
 bad:=[]string{
  "func F(a,b int) int { return a/b }",
  "func F(a,b int) int { println(a); return b }",
  "func F(a,b int) int { return a & b }",
  "func F(a string,b int) int { return b }",
  "func F(a,b int) int { for a>0 {a--}; return a+b }",
 }
 for _,src:=range bad {
  if _,err:=ASTCanonicalIntFunction(src);err==nil{t.Errorf("unsupported construct accepted: %s",src)}
 }
 yes,err:=ASTConservativeEquivalent(
  "func F(a,b int) int { return a+b }",
  "func F(x,y int) int { return y+x }")
 if err!=nil||!yes{t.Fatalf("commutative expression not recognized: equal=%t err=%v",yes,err)}
 no,err:=ASTConservativeEquivalent(
  "func F(a,b int) int { return a-b }",
  "func F(x,y int) int { return y-x }")
 if err!=nil||no{t.Fatalf("operand-order mutation wrongly certified: equal=%t err=%v",no,err)}
 big:=strings.Repeat("a+",2200)+"b"
 if _,err:=ASTCanonicalIntFunction("func F(a,b int) int {return "+big+"}");err==nil{t.Fatal("oversized AST input accepted")}
}

func TestASTBaselineUnknownDoesNotImplyDifference(t *testing.T){
 // These two forms are semantically identical for all integers, but
 // require a conditional rewrite this deliberately conservative baseline
 // does not yet implement. It must not claim inequivalence.
 a:="func F(a,b int) int { if a>b {return a};return b }"
 b:="func F(a,b int) int { if b>=a {return b};return a }"
 eq,err:=ASTConservativeEquivalent(a,b)
 if err!=nil{t.Fatal(err)}
 if eq{t.Fatal("this particular unsupported logical rewrite should not be accidentally canonicalized")}
 // The result is simply unknown, not "functionally changed".
}

func BenchmarkASTCanonicalGoFunction(b *testing.B){
 src:="func Weighted(a,b int) int { return a + (b * 2) }"
 b.ReportAllocs()
 for i:=0;i<b.N;i++ {
  v,err:=ASTCanonicalIntFunction(src)
  if err!=nil||len(v)==0||math.IsNaN(float64(len(v))){b.Fatal(err)}
 }
}
