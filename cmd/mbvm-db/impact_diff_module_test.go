package main

import (
 "bytes"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
)

func TestImpactDiffModuleRealContractCLI(t *testing.T){
 t.Setenv("GOWORK","off")
 t.Setenv("GOPROXY","off")
 t.Setenv("GOSUMDB","off")
 t.Setenv("CGO_ENABLED","0")
 root:=t.TempDir()
 oldRoot:=filepath.Join(root,"old")
 newRoot:=filepath.Join(root,"new")
 for _,v:=range []struct{dir,api,consumer string}{
  {oldRoot,
   "package api\nfunc Compute(x int)int{return x}\n",
   "package consumer\nimport \"example.test/clicontract/api\"\nfunc Use()int{return api.Compute(1)}\n"},
  {newRoot,
   "package api\nfunc Compute(x,y int)int{return x+y}\n",
   "package consumer\nimport \"example.test/clicontract/api\"\nfunc Use()int{return api.Compute(1,2)}\n"},
 }{
  for _,p:=range []string{"api","consumer"}{
   if err:=os.MkdirAll(filepath.Join(v.dir,p),0755);err!=nil{t.Fatal(err)}
  }
  files:=map[string]string{
   filepath.Join(v.dir,"go.mod"):"module example.test/clicontract\n\ngo 1.23\n",
   filepath.Join(v.dir,"api","api.go"):v.api,
   filepath.Join(v.dir,"consumer","consumer.go"):v.consumer,
  }
  for filename,content:=range files{
   if err:=os.WriteFile(filename,[]byte(content),0600);err!=nil{t.Fatal(err)}
  }
 }
 var out bytes.Buffer
 target:="example.test/clicontract/api.Compute"
 args:=[]string{"-module","-old",oldRoot,"-new",newRoot,"-symbol",target,"-json"}
 if err:=runImpactDiff(args,&out);err!=nil{t.Fatal(err)}
 var report struct{
  Schema string `json:"schema"`
  ContractChanged bool `json:"contract_changed"`
  SourceFilesUnmodified bool `json:"source_files_unmodified"`
  ConfirmedBrokenConsumers []string `json:"confirmed_broken_consumers"`
 }
 if err:=json.Unmarshal(out.Bytes(),&report);err!=nil{t.Fatal(err)}
 if report.Schema!="mbvm-real-module-diff-v1"||!report.ContractChanged||
  !report.SourceFilesUnmodified||len(report.ConfirmedBrokenConsumers)!=1{
  t.Fatalf("unexpected real version CLI JSON: %+v",report)
 }
 if err:=runImpactDiff([]string{"-module","-old",oldRoot,"-new",newRoot,"-json"},&out);err==nil{
  t.Fatal("accepted module diff without symbol")
 }
 if err:=runImpactDiff([]string{"-old",oldRoot,"-new",newRoot,"-symbol",target},&out);err==nil{
  t.Fatal("symbol incorrectly accepted without module mode")
 }
 if err:=runImpactDiff([]string{"-module","-old",oldRoot,"-new",newRoot,
  "-package","api","-symbol",target},&out);err==nil{
  t.Fatal("module+single-package flags incorrectly accepted")
 }
}
