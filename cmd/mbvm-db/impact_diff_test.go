package main

import (
 "bytes"
 "encoding/json"
 "os"
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
