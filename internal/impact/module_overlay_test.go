package impact

import (
 "bytes"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func internallyCallingModule(t *testing.T,makeProviderInvalid bool)string{
 t.Helper()
 root:=t.TempDir()
 for _,dir:=range []string{"api","client"}{
  if err:=os.MkdirAll(filepath.Join(root,dir),0755);err!=nil{t.Fatal(err)}
 }
 provider:=`package api
//go:noinline
func Run(x int) int { return x }
`
 if makeProviderInvalid{provider+="\nvar callback func(int) int = Run\n"}
 files:=map[string]string{
  filepath.Join(root,"go.mod"):"module example.test/overlaymodule\n\ngo 1.23\n",
  filepath.Join(root,"api","api.go"):provider,
  filepath.Join(root,"api","inside.go"):`package api
func Inside() int { return Run(10) }
`,
  filepath.Join(root,"client","client.go"):`package client
import "example.test/overlaymodule/api"
func Consumer() int { return api.Run(2) }
`,
 }
 for filename,contents:=range files{
  if err:=os.WriteFile(filename,[]byte(contents),0600);err!=nil{t.Fatal(err)}
 }
 return root
}

func TestPrepareContractOverlayAdaptsOnlyTypedProviderCalls(t *testing.T){
 t.Setenv("GOPROXY","off")
 t.Setenv("GOSUMDB","off")
 t.Setenv("GOWORK","off")
 root:=internallyCallingModule(t,false)
 graph,err:=LoadModule(root);if err!=nil{t.Fatal(err)}
 target:="example.test/overlaymodule/api.Run"
 provider,id,err:=graph.FindSymbol(target);if err!=nil{t.Fatal(err)}
 original:=map[string][]byte{}
 for _,file:=range provider.sourceFiles{
  content,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
  original[file]=content
 }
 prepared,err:=graph.PrepareContractOverlay(provider,id,t.TempDir())
 if err!=nil{t.Fatal(err)}
 if prepared.AdaptedInternalCalls!=1||len(prepared.ReplacedFiles)!=2{
  t.Fatalf("expected declaration and internal caller replacement: %+v",prepared)
 }
 for _,file:=range prepared.ReplacedFiles{
  originalBytes,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
  if !bytes.Equal(originalBytes,original[file]){
   t.Fatalf("original provider source modified: %s",file)
  }
 }
 data,err:=os.ReadFile(prepared.Path);if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(data),"overlay-"){t.Fatal("overlay map has no virtual replacement")}
 foundDirective,foundArgument:=false,false
 for i:=0;i<2;i++{
  content,err:=os.ReadFile(filepath.Join(filepath.Dir(prepared.Path),
   "overlay-00"+string(rune('0'+i))+".go"))
  if err!=nil{t.Fatal(err)}
  if strings.Contains(string(content),"//go:noinline"){foundDirective=true}
  if strings.Contains(string(content),"Run(10, 0)"){foundArgument=true}
 }
 if !foundDirective||!foundArgument{t.Fatal("overlay lost compiler directive or typed internal call adaptation")}
}

func TestSelfConsistentProviderCompilerGateRealConsumers(t *testing.T){
 t.Setenv("GOPROXY","off")
 t.Setenv("GOSUMDB","off")
 t.Setenv("GOWORK","off")
 root:=internallyCallingModule(t,false)
 graph,err:=LoadModule(root);if err!=nil{t.Fatal(err)}
 report,err:=graph.CheckCrossPackageBuild("example.test/overlaymodule/api.Run")
 if err!=nil{t.Fatal(err)}
 if !report.ProviderBaselineBuildPassed||!report.ProviderOverlayBuildPassed||
  report.AdaptedInternalCallSites!=1||!report.SourceFilesUnmodified{
  t.Fatalf("provider not self-consistent: %+v",report)
 }
 if len(report.ConfirmedBrokenConsumers)!=1||
  report.ConfirmedBrokenConsumers[0]!="example.test/overlaymodule/client"||
  len(report.InconclusiveConsumers)!=0{
  t.Fatalf("typed cross-package compile evidence missing: %+v",report)
 }
}

func TestProviderOverlayFailureLeavesConsumerInconclusive(t *testing.T){
 t.Setenv("GOPROXY","off")
 t.Setenv("GOSUMDB","off")
 t.Setenv("GOWORK","off")
 graph,err:=LoadModule(internallyCallingModule(t,true))
 if err!=nil{t.Fatal(err)}
 report,err:=graph.CheckCrossPackageBuild("example.test/overlaymodule/api.Run")
 if err!=nil{t.Fatal(err)}
 if !report.ProviderBaselineBuildPassed||report.ProviderOverlayBuildPassed||
  len(report.ConfirmedBrokenConsumers)!=0||len(report.InconclusiveConsumers)!=1||
  !report.SourceFilesUnmodified{
  t.Fatalf("provider failure must not be attributed to consumer: %+v",report)
 }
}

func TestKnownTargetCallLineDiagnosticAttribution(t *testing.T){
 m:=&ModuleGraph{
  Edges:[]ModuleEdge{{
   Caller:"example.test/test/client.Consumer",
   Callee:"example.test/test/api.Run",
   Position:"C:\\repo\\client\\client.go:7:22",
   CrossPackage:true,
  }},
 }
 target:="example.test/test/api.Run"
 caller:="example.test/test/client"
 if !m.matchesTargetCallDiagnostic(caller,target,[]string{
  "client\\client.go:7:34: not enough arguments in call to api.Run",
 }){
  t.Fatal("Windows diagnostic at statically resolved call site was not attributed")
 }
 for _,bad:=range [][]string{
  {"client\\client.go:9:34: not enough arguments in call to api.Run"},
  {"api\\api.go:7:34: not enough arguments in call to Run"},
  {"client\\client.go:7:34: unrelated type mismatch"},
 }{
  if m.matchesTargetCallDiagnostic(caller,target,bad){
   t.Fatalf("unrelated diagnostic incorrectly attributed: %v",bad)
  }
 }
}
