package main

import (
 "bytes"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
)

func TestImpactDiffCommandTwoDirectories(t *testing.T){
 root:=t.TempDir()
 oldDir,newDir:=filepath.Join(root,"v1"),filepath.Join(root,"v2")
 for _,dir:=range []string{oldDir,newDir}{
  if err:=os.MkdirAll(dir,0755);err!=nil{t.Fatal(err)}
 }
 oldSource:="package example\nfunc API(x int) int {return x}\nfunc Caller() int{return API(1)}\n"
 newSource:="package example\nfunc API(x int,y int) int{return x+y}\nfunc Caller() int{return API(1,2)}\n"
 for _,entry:=range [][2]string{{oldDir,oldSource},{newDir,newSource}}{
  if err:=os.WriteFile(filepath.Join(entry[0],"demo.go"),[]byte(entry[1]),0600);err!=nil{t.Fatal(err)}
 }
 var out bytes.Buffer
 err:=runImpactDiff([]string{"-old",oldDir,"-new",newDir,"-json"},&out)
 if err!=nil{t.Fatal(err)}
 var report struct{
  Schema string `json:"schema"`
  SourceFilesUnmodified bool `json:"source_files_unmodified"`
  Changes []struct{
   Symbol string `json:"symbol"`
   Kind string `json:"kind"`
   Compatibility *struct{
    Supported bool `json:"supported"`
    BrokenOldDirectCallers []string `json:"broken_old_direct_callers"`
   } `json:"old_consumer_compatibility"`
  } `json:"changes"`
 }
 if err:=json.Unmarshal(out.Bytes(),&report);err!=nil{t.Fatal(err)}
 if report.Schema!="mbvm-version-impact-v1"||!report.SourceFilesUnmodified{
  t.Fatalf("invalid JSON comparison result %+v",report)
 }
 witnessed:=false
 for _,change:=range report.Changes{
  if strings.HasSuffix(change.Symbol,".API"){
   if change.Kind!="signature_changed"||change.Compatibility==nil||
    !change.Compatibility.Supported||len(change.Compatibility.BrokenOldDirectCallers)!=1{
    t.Fatalf("no witnessed caller breakage: %+v",change)
   }
   witnessed=true
  }
 }
 if !witnessed{t.Fatal("expected changed API")}
 out.Reset()
 if err:=runImpactDiff([]string{"-new",newDir},&out);err==nil{
  t.Fatal("accepted missing old snapshot")
 }
 if err:=runImpactDiff([]string{"-old",oldDir,"-new",newDir,"unexpected"},&out);err==nil{
  t.Fatal("accepted positional argument")
 }
}

func TestImpactDiffGitRevisionFlags(t *testing.T){
 if _,err:=exec.LookPath("git");err!=nil{t.Skip("git unavailable")}
 root:=t.TempDir()
 pkg:=filepath.Join(root,"pkg")
 if err:=os.MkdirAll(pkg,0755);err!=nil{t.Fatal(err)}
 git:=func(args ...string){
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",root},args...)...)
  if result,err:=cmd.CombinedOutput();err!=nil{
   t.Fatalf("git %v: %v\n%s",args,err,result)
  }
 }
 git("init")
 path:=filepath.Join(pkg,"example.go")
 old:="package example\nfunc Get(x int)int{return x}\nfunc Use()int{return Get(1)}\n"
 newSource:="package example\nfunc Get(x,y int)int{return x+y}\nfunc Use()int{return Get(1,2)}\n"
 for n,src:=range []string{old,newSource}{
  if err:=os.WriteFile(path,[]byte(src),0600);err!=nil{t.Fatal(err)}
  git("add","pkg/example.go")
  git("-c","user.email=test@example.invalid","-c","user.name=MBVM Test","commit","-m",fmt.Sprintf("rev%d",n))
 }
 var out bytes.Buffer
 if err:=runImpactDiff([]string{"-repo",root,"-old-ref","HEAD~1","-new-ref","HEAD","-package","pkg","-json"},&out);err!=nil{t.Fatal(err)}
 if !strings.Contains(out.String(),`"signature_changed"`){
  t.Fatalf("Git revision command did not identify API change: %s",out.String())
 }
 out.Reset()
 if err:=runImpactDiff([]string{"-repo",root,"-old-ref","HEAD~1","-new-ref","HEAD","-old",pkg},&out);err==nil{
  t.Fatal("mixed revision/directory flags accepted")
 }
}
