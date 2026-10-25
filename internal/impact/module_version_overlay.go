package impact

import (
 "encoding/json"
 "fmt"
 "go/build"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

// overlayOldClient constructs a virtual client on top of the NEW tree,
// using the exact, unchanged Go source bytes from the OLD snapshot.
// It fails closed if the selected filename sets differ between snapshots.
func overlayOldClient(oldGraph *Graph,newDir,tmp string)(string,error){
 selected,err:=build.Default.ImportDir(newDir,0)
 if err!=nil{return "",fmt.Errorf("select new client sources: %w",err)}
 if len(selected.CgoFiles)>0{return "",fmt.Errorf("new client requires cgo")}
 oldNames:=make([]string,0,len(oldGraph.sourceFiles))
 for _,path:=range oldGraph.sourceFiles{oldNames=append(oldNames,filepath.Base(path))}
 sort.Strings(oldNames)
 newNames:=append([]string(nil),selected.GoFiles...)
 sort.Strings(newNames)
 if strings.Join(oldNames,"\x00")!=strings.Join(newNames,"\x00"){
  return "",fmt.Errorf("build-selected client file sets differ across revisions")
 }
 replacements:=map[string]string{}
 for i,name:=range oldNames{
  raw,err:=os.ReadFile(filepath.Join(oldGraph.Dir,name))
  if err!=nil{return "",err}
  destination:=filepath.Join(tmp,fmt.Sprintf("old-client-%03d.go",i))
  if err:=os.WriteFile(destination,raw,0600);err!=nil{return "",err}
  replacements[filepath.Join(newDir,name)]=destination
 }
 manifest,err:=json.Marshal(struct{Replace map[string]string `json:"Replace"`}{Replace:replacements})
 if err!=nil{return "",err}
 path:=filepath.Join(tmp,"old-client-overlay.json")
 if err:=os.WriteFile(path,manifest,0600);err!=nil{return "",err}
 return path,nil
}

// An imported call site was resolved in the OLD module and must be
// matched to the SAME relative client filename and line in the NEW tree.
// This is a compiler witness for the target, not a textual grep guess.
func oldClientTargetErrors(oldModule *ModuleGraph,consumer,target,newRoot string,diagnostics []string)[]string{
 found:=[]string{}
 for _,line:=range diagnostics{
  diagnosticFile,diagnosticLine,message,ok:=diagnosticLocation(line)
  if !ok{continue}
  if !strings.Contains(message,"not enough arguments in call to "){continue}
  functionName:=target[strings.LastIndex(target,".")+1:]
  reported:=strings.TrimSpace(strings.SplitN(message,"not enough arguments in call to ",2)[1])
  if reported!=functionName&&!strings.HasSuffix(reported,"."+functionName){continue}
  for _,edge:=range oldModule.Edges{
   if !edge.CrossPackage||edge.Callee!=target||
    !strings.HasPrefix(edge.Caller,consumer+"."){continue}
   oldSiteFile,oldLine,_,ok:=diagnosticLocation(edge.Position+": call")
   if !ok||oldLine!=diagnosticLine{continue}
   rel,err:=filepath.Rel(oldModule.Root,oldSiteFile)
   if err!=nil||!filepath.IsLocal(rel){continue}
   newSiteFile:=filepath.Join(newRoot,rel)
   if sameDiagnosticSource(newSiteFile,diagnosticFile){
    found=append(found,line)
    break
   }
  }
 }
 sort.Strings(found)
 return found
}
