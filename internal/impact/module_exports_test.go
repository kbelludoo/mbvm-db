package impact

import (
 "os"
 "path/filepath"
 "sort"
 "strings"
 "testing"
)

func graphSignature(g *Graph)([]string,[]string,[]string){
 symbols:=make([]string,0,len(g.Symbols))
 calls:=make([]string,0,len(g.Calls))
 imported:=make([]string,0,len(g.ImportedCalls))
 for _,s:=range g.Symbols{
  symbols=append(symbols,s.Qualified()+" "+s.Signature)
 }
 for _,call:=range g.Calls{
  calls=append(calls,g.Symbols[call.Caller].Qualified()+" -> "+g.Symbols[call.Callee].Qualified()+" @ "+call.Position)
 }
 for _,call:=range g.ImportedCalls{
  imported=append(imported,g.Symbols[call.Caller].Qualified()+" -> "+call.Callee+" @ "+call.Position)
 }
 sort.Strings(symbols);sort.Strings(calls);sort.Strings(imported)
 return symbols,calls,imported
}
func equalStrings(a,b []string)bool{
 return strings.Join(a,"\n")==strings.Join(b,"\n")
}

// The independent legacy per-package importer remains available through
// Load and loadWithPackagePath. Compare all statically typed calls and
// symbols, not merely the count of edges.
func TestSharedModuleImporterMatchesLegacyGraphs(t *testing.T){
 t.Setenv("GOPROXY","off");t.Setenv("GOSUMDB","off");t.Setenv("GOWORK","off")
 root:=moduleFixture(t)
 module,stats,err:=LoadModuleWithStats(root)
 if err!=nil{t.Fatal(err)}
 if stats.GoListCalls!=1||stats.Packages!=2||len(module.Packages)!=2{
  t.Fatalf("expected exactly one shared Go subprocess: %+v",stats)
 }
 if stats.ExportArchives==0||stats.TotalMilliseconds<=0{
  t.Fatalf("shared package export/timings not recorded: %+v",stats)
 }
 if stats.Symbols!=len(module.Symbols)||stats.Edges!=len(module.Edges){
  t.Fatalf("instrumentation disagrees with output: %+v",stats)
 }
 for _,shared:=range module.Packages{
  independent,err:=loadWithPackagePath(shared.Dir,shared.PackagePath)
  if err!=nil{t.Fatalf("legacy %s: %v",shared.PackagePath,err)}
  a,b,c:=graphSignature(shared)
  x,y,z:=graphSignature(independent)
  if !equalStrings(a,x)||!equalStrings(b,y)||!equalStrings(c,z)||
   shared.ExcludedCalls!=independent.ExcludedCalls{
   t.Fatalf("changed graph semantics for %s: shared %v %v %v; independent %v %v %v",
    shared.PackagePath,a,b,c,x,y,z)
  }
 }
 const callee="example.test/multipkg/api.(A).Apply"
 radius,err:=module.Impact(callee)
 if err!=nil||len(radius.DirectCallers)!=1||
  len(radius.TransitiveCandidates)!=1{
  t.Fatalf("shared importer changed multipackage CSR: %+v %v",radius,err)
 }
 t.Logf("shared: go list=%d, archives=%d, packages=%d, symbols=%d, edges=%d, discovery=%.3fms, typecheck=%.3fms, CSR=%.3fms, total=%.3fms",
  stats.GoListCalls,stats.ExportArchives,stats.Packages,stats.Symbols,
  stats.Edges,stats.DiscoveryMilliseconds,stats.TypecheckMilliseconds,
  stats.LinkMilliseconds,stats.TotalMilliseconds)
}

// No reusable global cache in this stage: re-running LoadModule MUST
// observe new source bytes rather than returning the earlier Graph.
func TestSharedModuleImporterReloadSeesSourceChange(t *testing.T){
 t.Setenv("GOPROXY","off");t.Setenv("GOSUMDB","off");t.Setenv("GOWORK","off")
 root:=moduleFixture(t)
 first,_,err:=LoadModuleWithStats(root)
 if err!=nil{t.Fatal(err)}
 api:=filepath.Join(root,"api","api.go")
 file,err:=os.OpenFile(api,os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{t.Fatal(err)}
 _,writeErr:=file.WriteString("\nfunc RecentlyAdded()int{return Run(1)}\n")
 closeErr:=file.Close()
 if writeErr!=nil||closeErr!=nil{t.Fatalf("modify fixture: %v %v",writeErr,closeErr)}
 second,stats,err:=LoadModuleWithStats(root)
 if err!=nil{t.Fatal(err)}
 if stats.GoListCalls!=1||len(second.Symbols)!=len(first.Symbols)+1||
  len(second.Edges)!=len(first.Edges)+1{
  t.Fatalf("stale module source after modification; %+v first=%d/%d second=%d/%d",
   stats,len(first.Symbols),len(first.Edges),len(second.Symbols),len(second.Edges))
 }
}

func BenchmarkLoadModuleFixture(b *testing.B){
 // Create one deterministic local, dependency-free module before
 // recording benchmark measurements, with multiple typed packages.
 root:=b.TempDir()
 for _,dir:=range []string{"api","client"}{
  if err:=os.MkdirAll(filepath.Join(root,dir),0755);err!=nil{b.Fatal(err)}
 }
 files:=map[string]string{
  filepath.Join(root,"go.mod"):"module example.test/benchmodule\n\ngo 1.23\n",
  filepath.Join(root,"api","api.go"):"package api\nfunc Run(x int)int{return x}\n",
  filepath.Join(root,"client","client.go"):"package client\nimport \"example.test/benchmodule/api\"\nfunc Use()int{return api.Run(1)}\n",
 }
 for filename,body:=range files{
  if err:=os.WriteFile(filename,[]byte(body),0600);err!=nil{b.Fatal(err)}
 }
 b.Setenv("GOPROXY","off");b.Setenv("GOSUMDB","off");b.Setenv("GOWORK","off")
 // Warm the compiler export cache, but not the analyzer graph itself.
 if _,_,err:=LoadModuleWithStats(root);err!=nil{b.Fatal(err)}
 b.ReportAllocs()
 b.ResetTimer()
 var last ModuleLoadStats
 for i:=0;i<b.N;i++{
  _,stats,err:=LoadModuleWithStats(root)
  if err!=nil{b.Fatal(err)}
  last=stats
 }
 b.StopTimer()
 b.ReportMetric(float64(last.GoListCalls),"go-list/op")
 b.ReportMetric(float64(last.Symbols),"symbols/op")
 b.ReportMetric(float64(last.Edges),"edges/op")
}

// Opt-in benchmark of the full real repository: it may trigger
// dependency compilation and is intentionally not run by default.
func BenchmarkLoadModuleReal(b *testing.B){
 if os.Getenv("MBVM_BENCH_REAL_MODULE")!="1"{
  b.Skip("set MBVM_BENCH_REAL_MODULE=1 for full-module benchmark")
 }
 root:="../.."
 if _,_,err:=LoadModuleWithStats(root);err!=nil{b.Fatal(err)}
 b.ReportAllocs()
 b.ResetTimer()
 var last ModuleLoadStats
 for i:=0;i<b.N;i++{
  _,stats,err:=LoadModuleWithStats(root)
  if err!=nil{b.Fatal(err)}
  last=stats
 }
 b.StopTimer()
 b.ReportMetric(float64(last.GoListCalls),"go-list/op")
 b.ReportMetric(float64(last.ExportArchives),"archives/op")
 b.ReportMetric(float64(last.Symbols),"symbols/op")
 b.ReportMetric(float64(last.Edges),"edges/op")
}
