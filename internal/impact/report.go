package impact

import "fmt"

// Report distinguishes static impact candidates from confirmed Go type
// errors after an explicitly requested simulated signature change.
type Report struct {
 Schema string `json:"schema"`
 Symbol string `json:"symbol"`
 Signature string `json:"signature"`
 PackagePath string `json:"package_path"`
 SourceSHA256 string `json:"source_sha256"`
 SourceFilesUnmodified bool `json:"source_files_unmodified"`
 TypedSymbols int `json:"typed_symbols"`
 LocalStaticCallSites int `json:"local_static_call_sites"`
 DirectCallers []string `json:"direct_callers"`
 TransitiveCallers []string `json:"transitive_callers"`
 TransitiveCandidates int `json:"transitive_candidates"`
 MaxDepth int `json:"max_depth"`
 Simulation *Simulation `json:"simulation,omitempty"`
 Warnings []string `json:"warnings"`
}

// Analyze handles one Go build-selected package, never entire dependency
// graphs or dynamic interface dispatch. It performs no source writes.
func Analyze(dir,query string,simulate bool)(Report,error){
 report:=Report{}
 graph,err:=Load(dir)
 if err!=nil{return report,err}
 id,err:=graph.Select(query)
 if err!=nil{return report,err}
 hash,err:=graph.SourceSHA256()
 if err!=nil{return report,err}
 reach,err:=graph.Reachable(id)
 if err!=nil{return report,err}
 symbol:=graph.Symbols[id]
 report=Report{
  Schema:"mbvm-impact-v1",Symbol:symbol.Qualified(),
  Signature:symbol.Signature,PackagePath:graph.PackagePath,
  SourceSHA256:hash,
  TypedSymbols:len(graph.Symbols),
  LocalStaticCallSites:len(graph.Calls),
  DirectCallers:[]string{},TransitiveCallers:[]string{},
  TransitiveCandidates:len(reach.Transitive),
  MaxDepth:reach.MaxDepth,
  Warnings:[]string{
   "Only one build-selected Go package is checked; callers in other packages are not included.",
   "Interface dispatch, reflection and unresolved indirect calls are not assigned to concrete methods.",
   "Transitive callers are candidates for impact, not demonstrated compilation errors.",
  },
 }
 for _,i:=range reach.Direct{report.DirectCallers=append(report.DirectCallers,graph.Symbols[i].Qualified())}
 for _,i:=range reach.Transitive{report.TransitiveCallers=append(report.TransitiveCallers,graph.Symbols[i].Qualified())}
 if simulate{
  simulation,err:=graph.SimulateAddIntParameter(id)
  if err!=nil{return Report{},fmt.Errorf("simulate contract mutation: %w",err)}
  report.Simulation=&simulation
  report.Warnings=append(report.Warnings,
   "Simulation mutates an AST in memory, not checked-in source; go/types errors are compile-time diagnostics only.")
 }
 after,err:=graph.SourceSHA256()
 if err!=nil{return Report{},err}
 if after!=hash{return Report{},fmt.Errorf("source files changed during impact analysis (before=%s after=%s)",hash,after)}
 report.SourceFilesUnmodified=true
 return report,nil
}
