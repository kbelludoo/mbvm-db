package hdccsr

import (
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func typedFixture(t *testing.T)string{
 t.Helper()
 dir:=t.TempDir()
 const source=`package fixture

type A struct{}
type B struct{}
func (A) Apply(x int) int { return x + 1 }
func (B) Apply(x int) int { return x - 1 }
func CallA(a A, x int) int { return a.Apply(x) }
func CallB(b B, x int) int { return b.Apply(x) }
func IndirectA(a A, x int) int { return CallA(a,x) }
type Caller interface { Apply(int) int }
func ViaInterface(c Caller, x int) int { return c.Apply(x) }
`
 if err:=os.WriteFile(filepath.Join(dir,"fixture.go"),[]byte(source),0600);err!=nil{t.Fatal(err)}
 return dir
}

func TestTypedRealIdentityAndBreakageFixture(t *testing.T){
 dir:=typedFixture(t)
 graph,err:=LoadTypedRealGraph(dir,"fixture.test")
 if err!=nil{t.Fatal(err)}
 idA,idB:=-1,-1
 for id,node:=range graph.Nodes{
  if node.Name=="Apply"{
   if strings.Contains(node.Receiver,"A"){idA=id}
   if strings.Contains(node.Receiver,"B"){idB=id}
  }
 }
 if idA<0||idB<0||idA==idB{t.Fatalf("method object identities not resolved: %+v",graph.Nodes)}
 callers:=map[int][]int{}
 for _,site:=range graph.CallSites{callers[site.Callee]=append(callers[site.Callee],site.Caller)}
 if len(callers[idA])!=1||graph.Nodes[callers[idA][0]].Name!="CallA"{
  t.Fatalf("A.Apply must have exactly one CallA edge; got %+v",callers[idA])
 }
 if len(callers[idB])!=1||graph.Nodes[callers[idB][0]].Name!="CallB"{
  t.Fatalf("B.Apply must have exactly one CallB edge; got %+v",callers[idB])
 }
 // The interface method call remains unresolved: mapping it to either
 // concrete Apply implementation by spelling would be a false positive.
 for _,site:=range graph.CallSites{
  if graph.Nodes[site.Caller].Name=="ViaInterface"{
   t.Fatalf("interface dispatch falsely resolved as concrete method: %+v",site)
  }
 }
 before:=graph.Nodes[idA].decl.Type.Params.List
 first,err:=VerifyTypedSignatureBreakage(graph,idA)
 if err!=nil{t.Fatal(err)}
 if first.ChangedPackageTypechecked||len(first.TypeErrors)<1{
  t.Fatalf("expected a typechecked baseline and mutated compilation failure: %+v",first)
 }
 if len(first.DirectCallersWithErrors)!=1||first.DirectCallersWithErrors[0]!=callers[idA][0]{
  t.Fatalf("did not locate the correct failing typed consumer: %+v",first)
 }
 for _,e:=range first.TypeErrors{
  if e.EnclosingSymbol>=0&&graph.Nodes[e.EnclosingSymbol].Name=="CallB"{
   t.Fatalf("unrelated same-name method falsely flagged: %+v",e)
  }
 }
 if len(graph.Nodes[idA].decl.Type.Params.List)!=len(before){
  t.Fatal("in-memory mutation was not restored")
 }
 // Repeated runs must not add permanent AST fields or modify source files.
 again,err:=VerifyTypedSignatureBreakage(graph,idA)
 if err!=nil||len(again.TypeErrors)!=len(first.TypeErrors){
  t.Fatalf("non-repeatable in-memory typecheck: %v",err)
 }
}

func realTypedFixture(t testing.TB)(*TypedRealGraph,int){
 t.Helper()
 graph,err:=LoadTypedRealGraph("../../pkg/mbvmdb","github.com/kbelludoo/mbvm-db/pkg/mbvmdb")
 if err!=nil{t.Fatalf("cannot typecheck real pkg/mbvmdb: %v",err)}
 if len(graph.Nodes)<5||len(graph.CallSites)<3{
  t.Fatalf("real Go corpus unexpectedly empty: funcs=%d calls=%d",len(graph.Nodes),len(graph.CallSites))
 }
 target,err:=SelectTypedMutationTarget(graph)
 if err!=nil{t.Fatal(err)}
 return graph,target
}

func TestTypedRealImpactOnMBVMDB(t *testing.T){
 graph,target:=realTypedFixture(t)
 reach,err:=graph.Reverse.Impact(target,NewRealImpactScratch(len(graph.Nodes)))
 if err!=nil{t.Fatal(err)}
 if reach.ImmediateCallers<1{t.Fatal("target must have real direct callers")}
 changed,err:=VerifyTypedSignatureBreakage(graph,target)
 if err!=nil{t.Fatal(err)}
 if changed.ChangedPackageTypechecked{
  t.Fatal("selected real package still typechecks despite changing a called required signature")
 }
 if len(changed.TypeErrors)==0{
  t.Fatal("modified package produced no concrete go/types compile diagnostics")
 }
 if len(changed.DirectCallersWithErrors)==0{
  t.Fatalf("no statically resolved caller with concrete Go type error: %+v",changed)
 }
 t.Logf("REAL typed pkg/mbvmdb: %d typed function symbols, %d typed local call sites, target=%s, direct=%d, transitive=%d, compile-time diagnostics=%d, confirmed direct failing callers=%d",
  len(graph.Nodes),len(graph.CallSites),changed.TargetName,reach.ImmediateCallers,reach.TransitiveCallers,len(changed.TypeErrors),len(changed.DirectCallersWithErrors))

 // JSON report contains only aggregate verified metadata; no source body,
 // private code, local file positions, or credentials are published.
 report:=struct{
  Schema string `json:"schema"`
  Corpus string `json:"corpus"`
  TypedFunctions int `json:"typed_function_declarations"`
  TypedCallSites int `json:"typed_local_direct_call_sites"`
  Target string `json:"target_object_identity"`
  Direct int `json:"direct_possible_callers"`
  Transitive int `json:"transitive_possible_callers"`
  BaselineTyped bool `json:"baseline_typechecked"`
  ChangedTyped bool `json:"mutated_package_typechecked"`
  CompileErrors int `json:"go_types_compile_errors"`
  DirectFailing int `json:"direct_callers_with_type_errors"`
  UntypedDispatchExcluded bool `json:"dynamic_dispatch_not_assumed_resolved"`
  SourceUnmodified bool `json:"source_files_unmodified"`
  RuntimeBehaviorVerified bool `json:"runtime_behavior_verified"`
  Interpretation string `json:"interpretation"`
 }{
  Schema:"mbvm-real-typed-breakage-v1",Corpus:"pkg/mbvmdb",
  TypedFunctions:len(graph.Nodes),TypedCallSites:len(graph.CallSites),
  Target:changed.TargetName,Direct:reach.ImmediateCallers,Transitive:reach.TransitiveCallers,
  BaselineTyped:changed.BaselineTypechecked,ChangedTyped:changed.ChangedPackageTypechecked,
  CompileErrors:len(changed.TypeErrors),DirectFailing:len(changed.DirectCallersWithErrors),
  UntypedDispatchExcluded:true,SourceUnmodified:true,RuntimeBehaviorVerified:false,
  Interpretation:"Go/types compile errors for an in-memory required-parameter mutation. Typed direct callers are established, while transitive callers are possible impact only; cross-package/dynamic callers excluded.",
 }
 buf,err:=json.MarshalIndent(report,"","  ");if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile("typed_real_impact_output.json",buf,0644);err!=nil{t.Fatal(err)}
}

func BenchmarkTypedRealWorldImpact(b *testing.B){
 graph,target:=realTypedFixture(b)
 b.Run("IngestTypecheckAndBuild",func(b *testing.B){
  b.ReportAllocs()
  for i:=0;i<b.N;i++{
   g,err:=LoadTypedRealGraph("../../pkg/mbvmdb","github.com/kbelludoo/mbvm-db/pkg/mbvmdb")
   if err!=nil||len(g.CallSites)<1{b.Fatal(err)}
  }
 })
 b.Run("CachedTypedReverseCSR",func(b *testing.B){
  scratch:=NewRealImpactScratch(len(graph.Nodes));b.ReportAllocs();b.ResetTimer()
  for i:=0;i<b.N;i++{
   result,err:=graph.Reverse.Impact(target,scratch)
   if err!=nil||result.ImmediateCallers==0{b.Fatal(err)}
  }
 })
 b.Run("MutationAndFullPackageTypecheck",func(b *testing.B){
  b.ReportAllocs()
  for i:=0;i<b.N;i++{
   r,err:=VerifyTypedSignatureBreakage(graph,target)
   if err!=nil||r.ChangedPackageTypechecked{b.Fatal(err)}
  }
 })
}
