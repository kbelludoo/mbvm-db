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

// oldClientOverlay records the reverse mapping from physical replacement
// files to their logical names in the new module's Go source tree.
// Go compiler diagnostics can use EITHER name depending on OS/toolchain.
type oldClientOverlay struct{
 Path string
 NewSourceByReplacement map[string]string
}

// overlayOldClient overlays exact OLD client bytes onto the NEW module.
// It fails closed when the build-selected filename sets differ.
func overlayOldClient(oldGraph *Graph,newDir,tmp string)(oldClientOverlay,error){
 out:=oldClientOverlay{NewSourceByReplacement:map[string]string{}}
 selected,err:=build.Default.ImportDir(newDir,0)
 if err!=nil{return out,fmt.Errorf("select new client sources: %w",err)}
 if len(selected.CgoFiles)>0{return out,fmt.Errorf("new client requires cgo")}
 oldNames:=make([]string,0,len(oldGraph.sourceFiles))
 for _,path:=range oldGraph.sourceFiles{oldNames=append(oldNames,filepath.Base(path))}
 sort.Strings(oldNames)
 newNames:=append([]string(nil),selected.GoFiles...)
 sort.Strings(newNames)
 if strings.Join(oldNames,"\x00")!=strings.Join(newNames,"\x00"){
  return out,fmt.Errorf("build-selected client file sets differ across revisions")
 }
 replacements:=map[string]string{}
 for i,name:=range oldNames{
  raw,err:=os.ReadFile(filepath.Join(oldGraph.Dir,name))
  if err!=nil{return out,err}
  destination:=filepath.Join(tmp,fmt.Sprintf("old-client-%03d.go",i))
  if err:=os.WriteFile(destination,raw,0600);err!=nil{return out,err}
  logical:=filepath.Join(newDir,name)
  replacements[logical]=destination
  out.NewSourceByReplacement[normalGoPath(destination)]=logical
 }
 manifest,err:=json.Marshal(struct{Replace map[string]string `json:"Replace"`}{Replace:replacements})
 if err!=nil{return out,err}
 out.Path=filepath.Join(tmp,"old-client-overlay.json")
 if err:=os.WriteFile(out.Path,manifest,0600);err!=nil{return out,err}
 return out,nil
}

// logicalSource resolves a compiler location to an exact overlay-mapped
// logical source path. The fallback requires the unique temporary run
// directory AND replacement filename, not a basename-only guess.
// It is deliberately limited to replacements in this ONE overlay.
func (o oldClientOverlay) logicalSource(diagnostic,newRoot string)(string,bool){
 d:=normalGoPath(diagnostic)
 for replacement,logical:=range o.NewSourceByReplacement{
  // Compiler may report the virtual Go source path.
  virtual:=normalGoPath(logical)
  if d==virtual||normalGoPath(filepath.Join(newRoot,diagnostic))==virtual{
   return logical,true
  }
  // Compiler may report the absolute physical replacement.
  if d==replacement||normalGoPath(filepath.Join(newRoot,diagnostic))==replacement{
   return logical,true
  }
  // Some Go/Windows diagnostics are relative to an internal build
  // directory rather than cmd.Dir, e.g. ..\\..\\mbvm-real-diff-XYZ\\
  // overlay-000\\old-client-000.go. Require the entire unique suffix.
  tmpDir:=filepath.Dir(replacement)
  runDir:=filepath.Base(filepath.Dir(tmpDir))
  suffix:="/"+normalGoPath(filepath.Join(runDir,filepath.Base(tmpDir),filepath.Base(replacement)))
  if strings.HasSuffix(d,suffix){return logical,true}
 }
 return "",false
}

// A compiler witness is accepted only if the diagnostic filename maps
// to a known virtual/physical overlay file AND a typed old call-site has
// the same logical relative path, source line and target function name.
func oldClientTargetErrors(oldModule *ModuleGraph,consumer,target,newRoot string,
 overlay oldClientOverlay,diagnostics []string)[]string{
 found:=[]string{}
 functionName:=target[strings.LastIndex(target,".")+1:]
 for _,line:=range diagnostics{
  diagnosticFile,diagnosticLine,message,ok:=diagnosticLocation(line)
  if !ok{continue}
  const prefix="not enough arguments in call to "
  if !strings.HasPrefix(message,prefix){continue}
  reported:=strings.TrimSpace(strings.TrimPrefix(message,prefix))
  if reported!=functionName&&!strings.HasSuffix(reported,"."+functionName){continue}
  logical,ok:=overlay.logicalSource(diagnosticFile,newRoot)
  if !ok{continue}
  for _,edge:=range oldModule.Edges{
   if !edge.CrossPackage||edge.Callee!=target||
    !strings.HasPrefix(edge.Caller,consumer+"."){continue}
   oldSiteFile,oldLine,_,ok:=diagnosticLocation(edge.Position+": call")
   if !ok||oldLine!=diagnosticLine{continue}
   rel,err:=filepath.Rel(oldModule.Root,oldSiteFile)
   if err!=nil||!filepath.IsLocal(rel){continue}
   newSiteFile:=filepath.Join(newRoot,rel)
   if normalGoPath(newSiteFile)==normalGoPath(logical){
    found=append(found,line)
    break
   }
  }
 }
 sort.Strings(found)
 return found
}
