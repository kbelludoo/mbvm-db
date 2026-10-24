package hdccsr

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "sort"
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

// Hash the exact source files selected by Go's build constraints. No
// temporary rewrite is permitted during this experiment.
func realTypedSourceDigest(g *TypedRealGraph)(string,error){
 var paths []string
 for _,f:=range g.Files{paths=append(paths,g.Fset.Position(f.Pos()).Filename)}
 sort.Strings(paths)
 h:=sha256.New()
 for _,path:=range paths{
  data,err:=os.ReadFile(path);if err!=nil{return "",err}
  h.Write([]byte(path))
  h.Write([]byte{0})
  h.Write(data)
  h.Write([]byte{0})
 }
 return hex.EncodeToString(h.Sum(nil)),nil
}

// Integration control for Go Modules resolution without network, GOPATH
// archives, protobuf, x/tools or private dependency caches. The external
// module is a local replace, but go list must resolve and compile its
// archive through the module-aware Go build system.
func TestTypedRealModuleAwareImportAndBreakage(t *testing.T){
 root:=t.TempDir()
 dep:=filepath.Join(root,"external")
 pkg:=filepath.Join(root,"pkg")
 if err:=os.MkdirAll(dep,0755);err!=nil{t.Fatal(err)}
 if err:=os.MkdirAll(pkg,0755);err!=nil{t.Fatal(err)}
 files:=map[string]string{
  filepath.Join(root,"go.mod"):"module example.test/parent\n\ngo 1.23\n\nrequire example.test/external v0.0.0\nreplace example.test/external => ./external\n",
  filepath.Join(dep,"go.mod"):"module example.test/external\n\ngo 1.23\n",
  filepath.Join(dep,"types.go"):"package external\ntype Thing struct { X int }\nfunc Value(a Thing) int { return a.X }\n",
  filepath.Join(pkg,"pkg.go"):"package pkg\nimport \"example.test/external\"\nfunc Resolve(v external.Thing) int { return external.Value(v) }\nfunc Consumer(v external.Thing) int { return Resolve(v) }\n",
 }
 for name,body:=range files{
  if err:=os.WriteFile(name,[]byte(body),0600);err!=nil{t.Fatal(err)}
 }
 graph,err:=LoadTypedRealGraph(pkg,"example.test/parent/pkg")
 if err!=nil{t.Fatalf("Go Modules import must resolve: %v",err)}
 target,err:=SelectTypedMutationTarget(graph)
 if err!=nil{t.Fatal(err)}
 if graph.Nodes[target].Name!="Resolve"{t.Fatalf("wrong mutation target: %s",graph.Nodes[target].Name)}
 result,err:=VerifyTypedSignatureBreakage(graph,target)
 if err!=nil{t.Fatal(err)}
 if result.ChangedPackageTypechecked||len(result.DirectCallersWithErrors)!=1||len(result.TypeErrors)<1{
  t.Fatalf("Go Modules consumer compile-time breakage not verified: %+v",result)
 }
 if graph.Nodes[result.DirectCallersWithErrors[0]].Name!="Consumer"{
  t.Fatalf("wrong direct breakage caller: %+v",result.DirectCallersWithErrors)
 }
}

func TestTypedRealModuleImporterFailsClosedWithoutDependency(t *testing.T){
 root:=t.TempDir()
 path:=filepath.Join(root,"broken.go")
 if err:=os.WriteFile(path,[]byte("package broken\nimport \"example.invalid/module/notinstalled\"\nfunc F() { notinstalled.Use() }\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=LoadTypedRealGraph(root,"example.test/broken");err==nil{
  t.Fatal("unresolved module import must fail instead of guessing/stubbing types")
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
 sourceBefore,err:=realTypedSourceDigest(graph);if err!=nil{t.Fatal(err)}
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
 sourceAfter,err:=realTypedSourceDigest(graph);if err!=nil{t.Fatal(err)}
 if sourceAfter!=sourceBefore{
  t.Fatal("the real Go source files changed during a supposedly in-memory-only contract mutation")
 }
 failing:=make([]string,0,len(changed.DirectCallersWithErrors))
 for _,id:=range changed.DirectCallersWithErrors{failing=append(failing,typedName(graph.Nodes[id]))}
 directNames:=make([]string,0,len(changed.DirectCallers))
 for _,id:=range changed.DirectCallers{directNames=append(directNames,typedName(graph.Nodes[id]))}
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
  SourceDigest string `json:"source_files_sha256"`
  TypedDirectCallers []string `json:"typed_direct_caller_names"`
  FailingCallers []string `json:"direct_callers_with_type_errors_names"`
  RuntimeBehaviorVerified bool `json:"runtime_behavior_verified"`
  Interpretation string `json:"interpretation"`
 }{
  Schema:"mbvm-real-typed-breakage-v1",Corpus:"pkg/mbvmdb",
  TypedFunctions:len(graph.Nodes),TypedCallSites:len(graph.CallSites),
  Target:changed.TargetName,Direct:reach.ImmediateCallers,Transitive:reach.TransitiveCallers,
  BaselineTyped:changed.BaselineTypechecked,ChangedTyped:changed.ChangedPackageTypechecked,
  CompileErrors:len(changed.TypeErrors),DirectFailing:len(changed.DirectCallersWithErrors),
  UntypedDispatchExcluded:true,SourceUnmodified:sourceAfter==sourceBefore,
  SourceDigest:sourceAfter,TypedDirectCallers:directNames,FailingCallers:failing,
  RuntimeBehaviorVerified:false,
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
