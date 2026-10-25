package impact

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

// Fixture uses a real Go module with distinct receiver-homonym targets,
// statically resolved cross-package clients, and interface dispatch.
func moduleFixture(t *testing.T)string{
 t.Helper()
 root:=t.TempDir()
 api:=filepath.Join(root,"api")
 client:=filepath.Join(root,"client")
 for _,d:=range []string{api,client}{if err:=os.MkdirAll(d,0755);err!=nil{t.Fatal(err)}}
 files:=map[string]string{
  filepath.Join(root,"go.mod"):"module example.test/multipkg\n\ngo 1.23\n",
  filepath.Join(api,"api.go"):`package api
type A struct{}
type B struct{}
func (A) Apply(x int) int{return x+1}
func (B) Apply(x int) int{return x+2}
func Run(x int) int{return x+1}
`,
  filepath.Join(client,"client.go"):`package client
import "example.test/multipkg/api"
func UseA(a api.A) int{return a.Apply(1)}
func UseB(b api.B) int{return b.Apply(1)}
func UseRun() int{return api.Run(2)}
func FromA(a api.A) int{return UseA(a)}
type Caller interface{Apply(int) int}
func Dynamic(v Caller)int{return v.Apply(1)}
`,
 }
 for p,body:=range files{if err:=os.WriteFile(p,[]byte(body),0600);err!=nil{t.Fatal(err)}}
 return root
}
func TestModuleGraphTypedCrossPackageCSR(t *testing.T){
 t.Setenv("GOPROXY","off");t.Setenv("GOSUMDB","off");t.Setenv("GOWORK","off")
 root:=moduleFixture(t)
 graph,err:=LoadModule(root);if err!=nil{t.Fatal(err)}
 if len(graph.Packages)!=2{t.Fatalf("expected 2 real Go packages, got %d",len(graph.Packages))}
 key:="example.test/multipkg/api.(A).Apply"
 impact,err:=graph.Impact(key);if err!=nil{t.Fatal(err)}
 if len(impact.DirectCallers)!=1||impact.DirectCallers[0]!="example.test/multipkg/client.UseA"||
  len(impact.TransitiveCandidates)!=1||
  impact.TransitiveCandidates[0]!="example.test/multipkg/client.FromA"{
  t.Fatalf("wrong typed cross-package reachability: %+v",impact)
 }
 if p:=graph.DirectConsumerPackages(key);len(p)!=1||p[0]!="example.test/multipkg/client"{
  t.Fatalf("wrong cross-package client %v",p)
 }
 b,err:=graph.Impact("example.test/multipkg/api.(B).Apply")
 if err!=nil||len(b.DirectCallers)!=1||strings.Contains(strings.Join(b.DirectCallers,","),"UseA"){
  t.Fatalf("same-name type method conflation: %+v %v",b,err)
 }
 // Interface method calls must never point to A or B concrete methods.
 for _,e:=range graph.Edges{
  if strings.Contains(e.Caller,"Dynamic")&&strings.HasSuffix(e.Callee,".Apply"){
   t.Fatalf("interface dispatch attributed to concrete method: %+v",e)
  }
 }
 t.Logf("typed multipackage graph: packages=%d functions=%d edges=%d cross-edges=%d",len(graph.Packages),len(graph.Symbols),len(graph.Edges),len(graph.DirectConsumerPackages(key)))
}
func TestCrossPackageOverlayActualCompilerWitness(t *testing.T){
 t.Setenv("GOPROXY","off");t.Setenv("GOSUMDB","off");t.Setenv("GOWORK","off")
 root:=moduleFixture(t)
 graph,err:=LoadModule(root);if err!=nil{t.Fatal(err)}
 target:="example.test/multipkg/api.Run"
 results,err:=graph.CheckCrossPackageBuild(target)
 if err!=nil{t.Fatal(err)}
 if !results.SourceFilesUnmodified||len(results.Consumers)!=1{
  t.Fatalf("bad source-integrity or consumer set %+v",results)
 }
 witness:=results.Consumers[0]
 if !witness.BaselineBuildPassed||witness.ChangedBuildPassed||
  !witness.ConfirmedConsumerError||len(results.ConfirmedBrokenConsumers)!=1{
  t.Fatalf("baseline/overlay Go compiler witness missing %+v",results)
 }
 if len(results.InconclusiveConsumers)!=0{t.Fatalf("unexpected inconclusive result: %+v",results)}
 t.Logf("verified cross-package caller compile failure: %s (%v)",witness.Package,witness.Diagnostics)
}
func TestModuleImpactRealMBVMDB(t *testing.T){
 // The real repository includes external Go Module imports (protobuf).
 // This test validates module discovery and actual consumers, not just
 // a synthetic module with local packages.
 module,err:=LoadModule("../..")
 if err!=nil{t.Fatal(err)}
 found:=false
 for _,e:=range module.Edges{
  if e.CrossPackage &&
    strings.Contains(e.Caller,"github.com/kbelludoo/mbvm-db/cmd/mbvm-db.")&&
    strings.Contains(e.Callee,"github.com/kbelludoo/mbvm-db/pkg/mbvmdb."){
   found=true;break
  }
 }
 if !found{t.Fatal("no concrete typed calls from MBVM-DB CLI to pkg/mbvmdb")}
 t.Logf("real MBVM module: packages=%d symbols=%d typed calls=%d",len(module.Packages),len(module.Symbols),len(module.Edges))
}
