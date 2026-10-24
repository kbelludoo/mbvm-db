package impact

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func fixture(t *testing.T)string{
 t.Helper()
 dir:=t.TempDir()
 const source=`package fixture
type A struct{}
type B struct{}
func (*A) Apply(x int) int { return x + 1 }
func (*B) Apply(x int) int { return x - 1 }
func Intern(x int) int { return x }
func (a *A) Intern(x int) int { return a.Apply(x) }
func CallA(a *A, x int) int { return a.Intern(x) }
func CallB(b *B, x int) int { return b.Apply(x) }
func ViaA(a *A, x int) int { return CallA(a,x) }
type Applicator interface { Apply(int) int }
func ViaInterface(a Applicator,x int) int { return a.Apply(x) }
`
 if err:=os.WriteFile(filepath.Join(dir,"fixture.go"),[]byte(source),0600);err!=nil{t.Fatal(err)}
 return dir
}
func TestSymbolIdentityAmbiguityAndBlastRadius(t *testing.T){
 dir:=fixture(t)
 g,err:=Load(dir);if err!=nil{t.Fatal(err)}
 if _,err:=g.Select("Intern");err==nil||!strings.Contains(err.Error(),"ambiguous"){
  t.Fatalf("bare homonym must be rejected with candidates: %v",err)
 }
 method,err:=g.Select("(*A).Intern");if err!=nil{t.Fatal(err)}
 fn,err:=g.Select(g.Symbols[method].Qualified());if err!=nil||fn!=method{t.Fatalf("qualified symbol resolution: %d %v",fn,err)}
 reach,err:=g.Reachable(method);if err!=nil{t.Fatal(err)}
 if len(reach.Direct)!=1||len(reach.Transitive)!=1||g.Symbols[reach.Direct[0]].Name!="CallA"||g.Symbols[reach.Transitive[0]].Name!="ViaA"{
  t.Fatalf("expected CallA direct and ViaA transitive, got %+v",reach)
 }
 if g.ExcludedCalls<1{t.Fatal("must count excluded interface dispatch")}
 for _,call:=range g.Calls{
  if g.Symbols[call.Caller].Name=="ViaInterface"{t.Fatal("interface call linked to arbitrary concrete method")}
 }
}

func TestInMemoryBreakageIntegrityAndRepeatability(t *testing.T){
 dir:=fixture(t)
 g,err:=Load(dir);if err!=nil{t.Fatal(err)}
 id,err:=g.Select("(*A).Intern");if err!=nil{t.Fatal(err)}
 before,err:=g.SourceSHA256();if err!=nil{t.Fatal(err)}
 first,err:=g.SimulateAddIntParameter(id);if err!=nil{t.Fatal(err)}
 if first.MutatedPackageTypechecked||len(first.BrokenDirectCallers)!=1||len(first.Diagnostics)==0{
  t.Fatalf("missing real Go diagnostics on direct CallA consumer: %+v",first)
 }
 if !strings.HasSuffix(first.BrokenDirectCallers[0],".CallA"){t.Fatalf("incorrect broken consumer %+v",first.BrokenDirectCallers)}
 second,err:=g.SimulateAddIntParameter(id)
 if err!=nil||len(second.Diagnostics)!=len(first.Diagnostics){t.Fatalf("not repeatable: %v %+v",err,second)}
 after,err:=g.SourceSHA256()
 if err!=nil||before!=after{t.Fatalf("source changed! %s vs %s: %v",before,after,err)}
 report,err:=Analyze(dir,"(*A).Intern",true)
 if err!=nil||!report.SourceFilesUnmodified||report.Simulation==nil{t.Fatalf("invalid analysis %+v %v",report,err)}
 if _,err:=Analyze(dir,"Intern",false);err==nil{t.Fatal("ambiguous symbol accepted by Analyze")}
}

func TestModuleImporterOfflineFixture(t *testing.T){
 t.Setenv("GOPROXY","off")
 t.Setenv("GOSUMDB","off")
 t.Setenv("GOWORK","off")
 root:=t.TempDir()
 dep:=filepath.Join(root,"external")
 pkg:=filepath.Join(root,"pkg")
 for _,dir:=range []string{dep,pkg}{if err:=os.MkdirAll(dir,0755);err!=nil{t.Fatal(err)}}
 files:=map[string]string{
  filepath.Join(root,"go.mod"):"module example.test/parent\n\ngo 1.23\n\nrequire example.test/external v0.0.0\nreplace example.test/external => ./external\n",
  filepath.Join(dep,"go.mod"):"module example.test/external\n\ngo 1.23\n",
  filepath.Join(dep,"dep.go"):"package external\ntype Thing struct{ Value int }\n",
  filepath.Join(pkg,"pkg.go"):"package pkg\nimport \"example.test/external\"\nfunc Resolve(x external.Thing) int {return x.Value}\nfunc Consumer(x external.Thing) int {return Resolve(x)}\n",
 }
 for filename,content:=range files{if err:=os.WriteFile(filename,[]byte(content),0600);err!=nil{t.Fatal(err)}}
 g,err:=Load(pkg);if err!=nil{t.Fatal(err)}
 if g.PackagePath!="example.test/parent/pkg"{t.Fatalf("module path mismatch %s",g.PackagePath)}
 r,err:=Analyze(pkg,"Resolve",true)
 if err!=nil||r.Simulation==nil||len(r.Simulation.BrokenDirectCallers)!=1{t.Fatalf("module-typed breakage: %+v %v",r,err)}
}

func TestRealMBVMDBPackage(t *testing.T){
 g,err:=Load("../../pkg/mbvmdb");if err!=nil{t.Fatal(err)}
 id,err:=g.Select("(*StringPool).Intern");if err!=nil{t.Fatal(err)}
 r,err:=Analyze("../../pkg/mbvmdb","(*StringPool).Intern",true)
 if err!=nil{t.Fatal(err)}
 if g.Symbols[id].Name!="Intern"||len(r.DirectCallers)!=4||len(r.TransitiveCallers)!=6{
  t.Fatalf("unexpected real typed callers in target package %+v",r)
 }
 if len(r.Simulation.BrokenDirectCallers)!=4||r.Simulation.MutatedPackageTypechecked{
  t.Fatalf("mutation didn't expose 4 real direct failures %+v",r.Simulation)
 }
 t.Logf("real MBVM impact: %s; direct=%d indirect=%d compile_diagnostics=%d",r.Symbol,len(r.DirectCallers),r.TransitiveCandidates,r.Simulation.TypeDiagnostics)
}

func TestCachedCSRTraversalDoesNotAllocate(t *testing.T){
 g,err:=Load(fixture(t));if err!=nil{t.Fatal(err)}
 id,err:=g.Select("(*A).Intern");if err!=nil{t.Fatal(err)}
 scratch:=NewScratch(len(g.Symbols))
 allocs:=testing.AllocsPerRun(100,func(){
  if _,err:=g.VisitCallers(id,scratch,nil);err!=nil{panic(err)}
 })
 if allocs!=0{t.Fatalf("cached reverse CSR should allocate 0, got %v",allocs)}
}

func BenchmarkImpactCachedReverseCSR(b *testing.B){
 g,err:=Load("../../pkg/mbvmdb");if err!=nil{b.Fatal(err)}
 id,err:=g.Select("(*StringPool).Intern");if err!=nil{b.Fatal(err)}
 scratch:=NewScratch(len(g.Symbols))
 b.ReportAllocs()
 b.ResetTimer()
 for i:=0;i<b.N;i++{
  if _,err:=g.VisitCallers(id,scratch,nil);err!=nil{b.Fatal(err)}
 }
}
func BenchmarkImpactMutationAndTypecheck(b *testing.B){
 g,err:=Load("../../pkg/mbvmdb");if err!=nil{b.Fatal(err)}
 id,err:=g.Select("(*StringPool).Intern");if err!=nil{b.Fatal(err)}
 b.ReportAllocs()
 b.ResetTimer()
 for i:=0;i<b.N;i++{
  report,err:=g.SimulateAddIntParameter(id)
  if err!=nil||report.MutatedPackageTypechecked{b.Fatal(err)}
 }
}
func BenchmarkImpactFullLoadQuery(b *testing.B){
 b.ReportAllocs()
 b.ResetTimer()
 for i:=0;i<b.N;i++{
  report,err:=Analyze("../../pkg/mbvmdb","(*StringPool).Intern",false)
  if err!=nil||len(report.DirectCallers)!=4{b.Fatal(err)}
 }
}
