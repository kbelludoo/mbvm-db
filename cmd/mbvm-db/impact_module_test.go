package main

import (
 "bytes"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
)

func TestImpactModuleCLIAndCompilerWitness(t *testing.T){
 t.Setenv("GOPROXY","off");t.Setenv("GOSUMDB","off");t.Setenv("GOWORK","off")
 root:=t.TempDir()
 api:=filepath.Join(root,"api");client:=filepath.Join(root,"client")
 for _,p:=range []string{api,client}{if err:=os.MkdirAll(p,0755);err!=nil{t.Fatal(err)}}
 fixtures:=map[string]string{
  filepath.Join(root,"go.mod"):"module example.test/cliimpact\n\ngo 1.23\n",
  filepath.Join(api,"api.go"):"package api\nfunc Run(x int)int{return x}\n",
  filepath.Join(client,"client.go"):"package client\nimport \"example.test/cliimpact/api\"\nfunc Consumer()int{return api.Run(1)}\n",
 }
 for p,c:=range fixtures{if err:=os.WriteFile(p,[]byte(c),0600);err!=nil{t.Fatal(err)}}
 args:=[]string{"-root",root,"-symbol","example.test/cliimpact/api.Run","-simulate-add-param","int","-json"}
 var out bytes.Buffer
 if err:=runImpactModule(args,&out);err!=nil{t.Fatal(err)}
 var report struct{
  Impact struct {
   Schema string `json:"schema"`
   DirectCallers []string `json:"direct_callers"`
  } `json:"impact"`
  Breakage struct{
   SourceFilesUnmodified bool `json:"source_files_unmodified"`
   ConfirmedBrokenConsumers []string `json:"confirmed_broken_consumers"`
  } `json:"cross_package_breakage"`
 }
 if err:=json.Unmarshal(out.Bytes(),&report);err!=nil{t.Fatal(err)}
 if report.Impact.Schema!="mbvm-module-impact-v1"||len(report.Impact.DirectCallers)!=1||
  !report.Breakage.SourceFilesUnmodified||len(report.Breakage.ConfirmedBrokenConsumers)!=1{
  t.Fatalf("CLI cross-package output invalid %+v",report)
 }
 out.Reset()
 if err:=runImpactModule([]string{"-root",root,"-symbol","Run","-json"},&out);err==nil{
  t.Fatal("bare ambiguous/nonqualified module identity accepted")
 }
 if err:=runImpactModule([]string{"-root",root,"-symbol","example.test/cliimpact/api.Run","-simulate-add-param","string"},&out);err==nil{
  t.Fatal("unsupported contract mutation accepted")
 }
 out.Reset()
 profiled:=append(append([]string(nil),args...),"-profile")
 if err:=runImpactModule(profiled,&out);err!=nil{t.Fatal(err)}
 var measured struct {
  LoadProfile *struct{
   GoListCalls int `json:"go_list_calls"`
   Packages int `json:"packages"`
   Symbols int `json:"symbols"`
   Edges int `json:"edges"`
   TotalMilliseconds float64 `json:"total_ms"`
  } `json:"load_profile"`
 }
 if err:=json.Unmarshal(out.Bytes(),&measured);err!=nil{t.Fatal(err)}
 if measured.LoadProfile==nil||measured.LoadProfile.GoListCalls!=1||
  measured.LoadProfile.Packages!=2||measured.LoadProfile.TotalMilliseconds<=0{
  t.Fatalf("missing or invalid one-process module profile %+v",measured)
 }
 out.Reset()
 if err:=runImpactModule([]string{"-root",root,"-symbol","example.test/cliimpact/api.Run","-json"},&out);err!=nil{t.Fatal(err)}
 var raw map[string]json.RawMessage
 if err:=json.Unmarshal(out.Bytes(),&raw);err!=nil{t.Fatal(err)}
 if _,exists:=raw["load_profile"];exists{
  t.Fatal("optional profiling changed default CLI JSON")
 }
}
