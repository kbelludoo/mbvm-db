package impact

import (
 "bytes"
 "fmt"
 "go/format"
 "go/types"
 "sort"
 "strings"
)

// ChangeKind is structural evidence, never proof of behavioral equivalence.
type ChangeKind string
const (
 Added ChangeKind="added"
 Removed ChangeKind="removed"
 SignatureChanged ChangeKind="signature_changed"
 BodyChanged ChangeKind="body_changed"
)

type VersionChange struct {
 Symbol string `json:"symbol"`
 Kind ChangeKind `json:"kind"`
 OldSignature string `json:"old_signature,omitempty"`
 NewSignature string `json:"new_signature,omitempty"`
 OldDirectCallers []string `json:"old_direct_callers"`
 OldTransitiveCandidates []string `json:"old_transitive_candidates"`
 Compatibility *CompatibilityCheck `json:"old_consumer_compatibility,omitempty"`
 Note string `json:"note"`
}

type VersionReport struct {
 Schema string `json:"schema"`
 OldPackage string `json:"old_package"`
 NewPackage string `json:"new_package"`
 OldSourceSHA256 string `json:"old_source_sha256"`
 NewSourceSHA256 string `json:"new_source_sha256"`
 SourceFilesUnmodified bool `json:"source_files_unmodified"`
 OldFunctionCount int `json:"old_function_count"`
 NewFunctionCount int `json:"new_function_count"`
 Changes []VersionChange `json:"changes"`
 Warnings []string `json:"warnings"`
}

// Symbol key is qualified by receiver, not merely its Go function name.
// Package path is checked for equality before comparison.
func versionKey(s Symbol)string{
 if s.Receiver!=""{return "("+s.Receiver+")."+s.Name}
 return s.Name
}

// Ignore parameter NAMES when comparing callable signatures.
// Fully qualified type names prevent same-name types in different imports
// from being declared identical. Generic constraints remain conservative.
func functionContract(s Symbol)(string,error){
 signature,ok:=s.object.Type().(*types.Signature)
 if !ok{return "",fmt.Errorf("missing typed signature for %s",s.Qualified())}
 qualifier:=func(p *types.Package)string{
  if p==nil{return ""}
  return p.Path()
 }
 typ:=func(t types.Type)string{return types.TypeString(t,qualifier)}
 var b strings.Builder
 if tp:=signature.TypeParams();tp!=nil&&tp.Len()>0 {
  b.WriteString("typeparams[")
  for i:=0;i<tp.Len();i++{
   if i>0{b.WriteByte(';')}
   b.WriteString(typ(tp.At(i).Constraint()))
  }
  b.WriteString("]")
 }
 b.WriteByte('(')
 for i:=0;i<signature.Params().Len();i++{
  if i>0{b.WriteByte(',')}
  b.WriteString(typ(signature.Params().At(i).Type()))
 }
 b.WriteString(")->(")
 for i:=0;i<signature.Results().Len();i++{
  if i>0{b.WriteByte(',')}
  b.WriteString(typ(signature.Results().At(i).Type()))
 }
 b.WriteByte(')')
 if signature.Variadic(){b.WriteString(":variadic")}
 return b.String(),nil
}

// This is a formatted AST/body fingerprint, not a proof of Go semantics.
func functionBody(g *Graph,id int)(string,error){
 var buf bytes.Buffer
 if err:=format.Node(&buf,g.fset,g.Symbols[id].decl.Body);err!=nil{return "",err}
 return buf.String(),nil
}

func (g *Graph) impactNames(id int)([]string,[]string,error){
 reach,err:=g.Reachable(id)
 if err!=nil{return nil,nil,err}
 direct:=make([]string,0,len(reach.Direct))
 transitive:=make([]string,0,len(reach.Transitive))
 for _,c:=range reach.Direct{direct=append(direct,g.Symbols[c].Qualified())}
 for _,c:=range reach.Transitive{transitive=append(transitive,g.Symbols[c].Qualified())}
 return direct,transitive,nil
}

// CompareDirs analyzes two independent, already healthy package snapshots.
// Both packages must have the SAME import path; a different version's
// module imports/configuration may invalidate in-memory compatibility checks.
// No source trees are edited or checked out by this function.
func CompareDirs(oldDir,newDir string)(VersionReport,error){
 report:=VersionReport{Schema:"mbvm-version-impact-v1",Changes:[]VersionChange{},
  Warnings:[]string{
   "Structural source change is not a demonstrated functional change.",
   "Compatibility checking uses OLD same-package callers against a NEW function signature, not all external clients.",
   "Go build tags/GOOS/GOARCH, interface dispatch, reflection and function values are not exhaustively analyzed.",
   "Removed functions and body-only changes have no compile-time compatibility witness in this release.",
  },
 }
 oldGraph,err:=Load(oldDir);if err!=nil{return report,fmt.Errorf("load old package: %w",err)}
 newGraph,err:=Load(newDir);if err!=nil{return report,fmt.Errorf("load new package: %w",err)}
 if oldGraph.PackagePath!=newGraph.PackagePath{
  return report,fmt.Errorf("cannot compare different package import paths: %q vs %q",oldGraph.PackagePath,newGraph.PackagePath)
 }
 oldHash,err:=oldGraph.SourceSHA256();if err!=nil{return report,err}
 newHash,err:=newGraph.SourceSHA256();if err!=nil{return report,err}
 report.OldPackage=oldGraph.PackagePath
 report.NewPackage=newGraph.PackagePath
 report.OldSourceSHA256=oldHash
 report.NewSourceSHA256=newHash
 report.OldFunctionCount=len(oldGraph.Symbols)
 report.NewFunctionCount=len(newGraph.Symbols)
 oldIndex,newIndex:=make(map[string]int),make(map[string]int)
 keys:=map[string]struct{}{}
 for i,s:=range oldGraph.Symbols{
  key:=versionKey(s)
  if _,exists:=oldIndex[key];exists{return report,fmt.Errorf("duplicate old symbol key %s",key)}
  oldIndex[key]=i
  keys[key]=struct{}{}
 }
 for i,s:=range newGraph.Symbols{
  key:=versionKey(s)
  if _,exists:=newIndex[key];exists{return report,fmt.Errorf("duplicate new symbol key %s",key)}
  newIndex[key]=i
  keys[key]=struct{}{}
 }
 sorted:=make([]string,0,len(keys))
 for key:=range keys{sorted=append(sorted,key)}
 sort.Strings(sorted)
 for _,key:=range sorted{
  oi,oldExists:=oldIndex[key]
  ni,newExists:=newIndex[key]
  change:=VersionChange{
   Symbol:oldGraph.PackagePath+"."+key,
   OldDirectCallers:[]string{},OldTransitiveCandidates:[]string{},
  }
  if oldExists{
   change.OldSignature=oldGraph.Symbols[oi].Signature
   change.OldDirectCallers,change.OldTransitiveCandidates,err=oldGraph.impactNames(oi)
   if err!=nil{return report,err}
  }
  if newExists{change.NewSignature=newGraph.Symbols[ni].Signature}
  switch{
  case !oldExists:
   change.Kind=Added
   change.Note="New declaration; there are no old callers of this declaration to certify."
  case !newExists:
   change.Kind=Removed
   change.Note="Declaration removed. Old callers are candidates; removal has not been type-checked against old consumer code."
  default:
   oldContract,err:=functionContract(oldGraph.Symbols[oi]);if err!=nil{return report,err}
   newContract,err:=functionContract(newGraph.Symbols[ni]);if err!=nil{return report,err}
   if oldContract!=newContract{
    change.Kind=SignatureChanged
    change.Note="Callable type contract changed; old same-package consumer compatibility is checked when safely supported."
    check:=CheckOldConsumersWithNewSignature(oldGraph,oi,newGraph,ni)
    change.Compatibility=&check
   }else{
    oldBody,err:=functionBody(oldGraph,oi);if err!=nil{return report,err}
    newBody,err:=functionBody(newGraph,ni);if err!=nil{return report,err}
    if oldBody==newBody{continue}
    change.Kind=BodyChanged
    change.Note="Function body AST changed; neither equivalence nor a behavior difference has been certified."
   }
  }
  report.Changes=append(report.Changes,change)
 }
 afterOld,err:=oldGraph.SourceSHA256();if err!=nil{return report,err}
 afterNew,err:=newGraph.SourceSHA256();if err!=nil{return report,err}
 if afterOld!=oldHash||afterNew!=newHash{
  return report,fmt.Errorf("Go source files changed during version comparison")
 }
 report.SourceFilesUnmodified=true
 return report,nil
}
