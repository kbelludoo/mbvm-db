package impact

import (
 "path/filepath"
 "regexp"
 "strconv"
 "strings"
)

// Go compiler diagnostics have a source file and line (optionally column).
// Greedy .go capture also handles Windows drive-letter paths.
var compilerLocation=regexp.MustCompile(`^(.+\.go):([0-9]+)(?::[0-9]+)?:\s*(.*)$`)

func diagnosticLocation(text string)(filename string,line int,message string,ok bool){
 match:=compilerLocation.FindStringSubmatch(strings.TrimSpace(text))
 if match==nil{return "",0,"",false}
 number,err:=strconv.Atoi(match[2])
 if err!=nil||number<=0{return "",0,"",false}
 return match[1],number,match[3],true
}
func normalGoPath(filename string)string{
 clean:=strings.ReplaceAll(filename,"\\","/")
 return strings.ToLower(filepath.ToSlash(filepath.Clean(clean)))
}
func sameDiagnosticSource(expected,actual string)bool{
 a,b:=normalGoPath(expected),normalGoPath(actual)
 return a==b||strings.HasSuffix(a,"/"+b)||strings.HasSuffix(b,"/"+a)
}

// matchesTargetCallDiagnostic requires an observed compiler failure at
// an actual cross-package CALL SITE that go/types linked to this target.
// A different error elsewhere in the consumer must remain inconclusive.
func (m *ModuleGraph) matchesTargetCallDiagnostic(consumer,target string,lines []string)bool{
 for _,line:=range lines{
  file,number,message,ok:=diagnosticLocation(line)
  if !ok||!strings.Contains(message,"not enough arguments in call"){continue}
  for _,edge:=range m.Edges{
   if !edge.CrossPackage||edge.Callee!=target||
    !strings.HasPrefix(edge.Caller,consumer+"."){continue}
   siteFile,siteLine,_,ok:=diagnosticLocation(edge.Position+": source")
   if !ok{continue}
   if number==siteLine&&sameDiagnosticSource(siteFile,file){return true}
  }
 }
 return false
}
