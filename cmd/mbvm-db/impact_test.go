package main

import (
 "bytes"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestImpactCommandJSONAndArgumentRejection(t *testing.T){
 root:=t.TempDir()
 source:=`package fixture
type Pool struct{}
func (p *Pool) Intern(x int) int { return x }
func Caller(p *Pool, x int) int {return p.Intern(x)}
`
 if err:=os.WriteFile(filepath.Join(root,"fixture.go"),[]byte(source),0600);err!=nil{t.Fatal(err)}
 var out bytes.Buffer
 args:=[]string{"-dir",root,"-symbol","(*Pool).Intern","-simulate-add-param","int","-json"}
 if err:=runImpact(args,&out);err!=nil{t.Fatal(err)}
 var report struct{
  Schema string `json:"schema"`
  Symbol string `json:"symbol"`
  SourceFilesUnmodified bool `json:"source_files_unmodified"`
  DirectCallers []string `json:"direct_callers"`
  Simulation struct{
   BrokenDirectCallers []string `json:"broken_direct_callers"`
  } `json:"simulation"`
 }
 if err:=json.Unmarshal(out.Bytes(),&report);err!=nil{t.Fatal(err)}
 if report.Schema!="mbvm-impact-v1"||!report.SourceFilesUnmodified||
  len(report.DirectCallers)!=1||len(report.Simulation.BrokenDirectCallers)!=1{
  t.Fatalf("unexpected JSON report %+v",report)
 }
 out.Reset()
 if err:=runImpact([]string{"-dir",root,"-symbol","Intern","-simulate-add-param","string"},&out);err==nil{t.Fatal("unsupported mutation silently accepted")}
 if err:=runImpact([]string{"-dir",root,"-symbol",""},&out);err==nil{t.Fatal("missing symbol accepted")}
 if strings.Contains(out.String(),"{"){t.Fatal("failed commands should not print JSON")}
}
