package impact

import (
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
)

const realDiffTarget="example.test/realdiff/api.Run"
func realModuleFixture(t *testing.T,oldAPI,newAPI,oldClient,newClient string)(string,string){
 t.Helper()
 root:=t.TempDir()
 oldRoot:=filepath.Join(root,"old")
 newRoot:=filepath.Join(root,"new")
 for _,r:=range []string{oldRoot,newRoot}{
  for _,p:=range []string{"api","client"}{
   if err:=os.MkdirAll(filepath.Join(r,p),0755);err!=nil{t.Fatal(err)}
  }
  if err:=os.WriteFile(filepath.Join(r,"go.mod"),[]byte("module example.test/realdiff\n\ngo 1.23\n"),0600);err!=nil{t.Fatal(err)}
 }
 for _,x:=range [][2]string{
  {filepath.Join(oldRoot,"api","api.go"),oldAPI},
  {filepath.Join(newRoot,"api","api.go"),newAPI},
  {filepath.Join(oldRoot,"client","client.go"),oldClient},
  {filepath.Join(newRoot,"client","client.go"),newClient},
 }{
  if err:=os.WriteFile(x[0],[]byte(x[1]),0600);err!=nil{t.Fatal(err)}
 }
 return oldRoot,newRoot
}
func setRealDiffOffline(t *testing.T){
 t.Helper()
 t.Setenv("GOWORK","off")
 t.Setenv("GOPROXY","off")
 t.Setenv("GOSUMDB","off")
 t.Setenv("CGO_ENABLED","0")
}
func TestRealModuleDiffOldConsumerBreaksOnNewContract(t *testing.T){
 setRealDiffOffline(t)
 oldRoot,newRoot:=realModuleFixture(t,
  "package api\nfunc Run(x int) int{return x}\n",
  "package api\nfunc Run(x,y int) int{return x+y}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2,3)}\n",
 )
 report,err:=CompareRealModuleDirs(oldRoot,newRoot,realDiffTarget)
 if err!=nil{t.Fatal(err)}
 if report.Schema!="mbvm-real-module-diff-v1"||!report.ContractChanged||
  !report.ProviderBaselinePassed||!report.SourceFilesUnmodified||
  len(report.ConfirmedBrokenConsumers)!=1||len(report.InconclusiveConsumers)!=0{
  t.Fatalf("real compiler diff should identify old caller break: %+v",report)
 }
 c:=report.Consumers[0]
 if !c.OldBaselinePassed||!c.NewBaselinePassed||c.OldClientWithNewProviderPassed||
  c.Status!="broken"||len(c.ConfirmedCompileErrors)!=1{
  t.Fatalf("missing old-client compiler witness %+v",c)
 }
 t.Logf("old client/new real provider: %v",c.ConfirmedCompileErrors)
}
func TestRealModuleDiffCompatibleOldClient(t *testing.T){
 setRealDiffOffline(t)
 oldRoot,newRoot:=realModuleFixture(t,
  "package api\nfunc Run(x int) int{return x}\n",
  "package api\nfunc Run(x any) int{return 2}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
 )
 result,err:=CompareRealModuleDirs(oldRoot,newRoot,realDiffTarget)
 if err!=nil{t.Fatal(err)}
 if !result.ContractChanged||len(result.CompatibleInTest)!=1||
  len(result.ConfirmedBrokenConsumers)!=0||result.Consumers[0].Status!="compatible_in_test"{
  t.Fatalf("compatible old client misclassified: %+v",result)
 }
}
func TestRealModuleDiffFileSetChangeInconclusive(t *testing.T){
 setRealDiffOffline(t)
 oldRoot,newRoot:=realModuleFixture(t,
  "package api\nfunc Run(x int) int{return x}\n",
  "package api\nfunc Run(x,y int) int{return x+y}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2,3)}\n",
 )
 if err:=os.WriteFile(filepath.Join(newRoot,"client","extra.go"),
  []byte("package client\nfunc Extra(){}\n"),0600);err!=nil{t.Fatal(err)}
 r,err:=CompareRealModuleDirs(oldRoot,newRoot,realDiffTarget)
 if err!=nil{t.Fatal(err)}
 if len(r.InconclusiveConsumers)!=1||len(r.ConfirmedBrokenConsumers)!=0||
  !strings.Contains(r.Consumers[0].Reason,"file sets differ"){
  t.Fatalf("file set drift must fail closed: %+v",r)
 }
}
func TestRealModuleGitRefsCompilerEvidenceNoCheckout(t *testing.T){
 setRealDiffOffline(t)
 if _,err:=exec.LookPath("git");err!=nil{t.Skip("git executable missing")}
 oldRoot,newRoot:=realModuleFixture(t,
  "package api\nfunc Run(x int) int{return x}\n",
  "package api\nfunc Run(x,y int) int{return x+y}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2,3)}\n",
 )
 repo:=t.TempDir()
 gitFixtureRun(t,repo,"init")
 for n,source:=range []string{oldRoot,newRoot}{
  for _,rel:=range []string{"go.mod","api/api.go","client/client.go"}{
   from,err:=os.ReadFile(filepath.Join(source,filepath.FromSlash(rel)));if err!=nil{t.Fatal(err)}
   target:=filepath.Join(repo,filepath.FromSlash(rel))
   if err:=os.MkdirAll(filepath.Dir(target),0755);err!=nil{t.Fatal(err)}
   if err:=os.WriteFile(target,from,0600);err!=nil{t.Fatal(err)}
  }
  gitFixtureRun(t,repo,"add",".")
  gitFixtureRun(t,repo,"-c","user.name=MBVM Test","-c","user.email=test@example.invalid","commit","-m",string(rune('1'+n)))
 }
 dirt:=filepath.Join(repo,"dirty.txt")
 if err:=os.WriteFile(dirt,[]byte("unchanged"),0600);err!=nil{t.Fatal(err)}
 result,err:=CompareRealModuleGitRefs(repo,"HEAD~1","HEAD",realDiffTarget)
 if err!=nil{t.Fatal(err)}
 if !result.SourceFilesUnmodified||len(result.ConfirmedBrokenConsumers)!=1{
  t.Fatalf("real Git ref compiler witness: %+v",result)
 }
 if got,err:=os.ReadFile(dirt);err!=nil||string(got)!="unchanged"{
  t.Fatal("Git comparison changed untracked file")
 }
}

func TestRealModuleDiffReturnTypeChangeRemainsInconclusive(t *testing.T){
 setRealDiffOffline(t)
 oldRoot,newRoot:=realModuleFixture(t,
  "package api\nfunc Run(x int) int{return x}\n",
  "package api\nfunc Run(x int) string{return \"new\"}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() string{return api.Run(2)}\n",
 )
 result,err:=CompareRealModuleDirs(oldRoot,newRoot,realDiffTarget)
 if err!=nil{t.Fatal(err)}
 if !result.ContractChanged||len(result.InconclusiveConsumers)!=1||
  len(result.ConfirmedBrokenConsumers)!=0||result.Consumers[0].Status!="inconclusive"{
  t.Fatalf("unsupported return-type error must not be overclaimed: %+v",result)
 }
}
func TestRealModuleDiffUnchangedContractNotReportedAsBreak(t *testing.T){
 setRealDiffOffline(t)
 oldRoot,newRoot:=realModuleFixture(t,
  "package api\nfunc Run(x int) int{return x}\n",
  "package api\nfunc Run(x int) int{return x+1}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use() int{return api.Run(2)}\n",
 )
 result,err:=CompareRealModuleDirs(oldRoot,newRoot,realDiffTarget)
 if err!=nil{t.Fatal(err)}
 if result.ContractChanged||len(result.ConfirmedBrokenConsumers)!=0||
  len(result.InconclusiveConsumers)!=0||!result.SourceFilesUnmodified{
  t.Fatalf("body-only change incorrectly reported as signature break: %+v",result)
 }
}

func TestOldClientOverlayDiagnosticReverseMapping(t *testing.T){
 setRealDiffOffline(t)
 oldRoot,newRoot:=realModuleFixture(t,
  "package api\nfunc Run(x int)int{return x}\n",
  "package api\nfunc Run(x,y int)int{return x+y}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use()int{return api.Run(2)}\n",
  "package client\nimport \"example.test/realdiff/api\"\nfunc Use()int{return api.Run(2,3)}\n",
 )
 oldModule,err:=LoadModule(oldRoot)
 if err!=nil{t.Fatal(err)}
 oldGraph,_,err:=oldModule.FindSymbol("example.test/realdiff/client.Use")
 if err!=nil{t.Fatal(err)}
 tmp:=filepath.Join(t.TempDir(),"overlay-000")
 if err:=os.MkdirAll(tmp,0700);err!=nil{t.Fatal(err)}
 overlay,err:=overlayOldClient(oldGraph,filepath.Join(newRoot,"client"),tmp)
 if err!=nil{t.Fatal(err)}
 if len(overlay.NewSourceByReplacement)!=1{t.Fatalf("expected one overlay replacement: %+v",overlay)}
 var physical,logical string
 for p,l:=range overlay.NewSourceByReplacement{physical,logical=p,l}
 reference:=":3:31: not enough arguments in call to api.Run"
 windowsRelative:="..\\..\\..\\"+filepath.Base(filepath.Dir(tmp))+"\\"+
  filepath.Base(tmp)+"\\"+filepath.Base(physical)
 cases:=[]struct{label,location string;want bool}{
  {"physical absolute",physical,true},
  {"logical virtual",logical,true},
  {"Windows physical relative",windowsRelative,true},
  {"wrong caller line",physical[:len(physical)],false},
  {"unrelated physical overlay",filepath.Join(t.TempDir(),"overlay-000","old-client-000.go"),false},
 }
 for _,tc:=range cases{
  message:=tc.location+reference
  if tc.label=="wrong caller line"{
   message=tc.location+":4:31: not enough arguments in call to api.Run"
  }
  got:=oldClientTargetErrors(oldModule,"example.test/realdiff/client",realDiffTarget,
   newRoot,overlay,[]string{message})
  if (len(got)>0)!=tc.want{
   t.Fatalf("%s: want match %v, got %v from %q",tc.label,tc.want,got,message)
  }
 }
 for _,message:=range []string{
  physical+":3:31: not enough arguments in call to api.Other",
  physical+":3:31: cannot use string as int",
 }{
  if got:=oldClientTargetErrors(oldModule,"example.test/realdiff/client",realDiffTarget,
   newRoot,overlay,[]string{message});len(got)>0{
   t.Fatalf("unrelated compiler error incorrectly confirmed: %v",got)
  }
 }
}
