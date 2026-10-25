package impact

import (
 "bytes"
 "encoding/json"
 "fmt"
 "io"
 "os/exec"
 "path/filepath"
 "sort"
 "strings"
)

// ModuleGraph merges separately typechecked local Go packages using stable,
// fully qualified function and receiver identifiers. A matched imported
// call is a statically resolved dependency, not proof of compile breakage.
type ModuleGraph struct{
 Root string
 Packages []*Graph
 PackageDirs map[string]string
 Symbols []string
 Edges []ModuleEdge
 Offsets []uint32
 Callers []uint32
 UnlinkedImports int
}
type ModuleEdge struct{
 Caller string `json:"caller"`
 Callee string `json:"callee"`
 Position string `json:"position"`
 CrossPackage bool `json:"cross_package"`
}
type ModuleImpact struct{
 Schema string `json:"schema"`
 Symbol string `json:"symbol"`
 Packages int `json:"packages"`
 Symbols int `json:"symbols"`
 ResolvedEdges int `json:"resolved_edges"`
 UnlinkedImports int `json:"unlinked_imported_call_sites"`
 DirectCallers []string `json:"direct_callers"`
 TransitiveCandidates []string `json:"transitive_candidates"`
 MaxDepth int `json:"max_depth"`
 Warnings []string `json:"warnings"`
}

type goListPackage struct{
 ImportPath string
 Dir string
 Error *struct{Err string}
}

func discoverModulePackages(root string)([]goListPackage,error){
 cmd:=exec.Command("go","list","-mod=readonly","-json","./...")
 cmd.Dir=root
 var stdout,stderr bytes.Buffer
 cmd.Stdout=&stdout;cmd.Stderr=&stderr
 if err:=cmd.Run();err!=nil{
  return nil,fmt.Errorf("go list -mod=readonly -json ./...: %w: %s",err,strings.TrimSpace(stderr.String()))
 }
 decoder:=json.NewDecoder(bytes.NewReader(stdout.Bytes()))
 out:=[]goListPackage{}
 for{
  var pkg goListPackage
  if err:=decoder.Decode(&pkg);err!=nil{
   if err==io.EOF{break}
   return nil,fmt.Errorf("decode go list: %w",err)
  }
  if pkg.Error!=nil{return nil,fmt.Errorf("package %s: %s",pkg.ImportPath,pkg.Error.Err)}
  if pkg.Dir==""||pkg.ImportPath==""{return nil,fmt.Errorf("go list missing package identity")}
  out=append(out,pkg)
  if len(out)>2048{return nil,fmt.Errorf("module exceeds 2048 package limit")}
 }
 sort.Slice(out,func(i,j int)bool{return out[i].ImportPath<out[j].ImportPath})
 return out,nil
}

func LoadModule(root string)(*ModuleGraph,error){
 abs,err:=filepath.Abs(root);if err!=nil{return nil,err}
 pkgs,err:=discoverModulePackages(abs);if err!=nil{return nil,err}
 if len(pkgs)==0{return nil,fmt.Errorf("no Go packages found in %s",abs)}
 m:=&ModuleGraph{Root:abs,PackageDirs:map[string]string{},Symbols:[]string{},Edges:[]ModuleEdge{}}
 bySymbol:=map[string]int{}
 for _,pkg:=range pkgs{
  g,err:=loadWithPackagePath(pkg.Dir,pkg.ImportPath)
  if err!=nil{return nil,fmt.Errorf("load module package %s: %w",pkg.ImportPath,err)}
  m.Packages=append(m.Packages,g)
  m.PackageDirs[pkg.ImportPath]=pkg.Dir
  for _,symbol:=range g.Symbols{
   key:=symbol.Qualified()
   if _,ok:=bySymbol[key];ok{return nil,fmt.Errorf("duplicate qualified function identity %s",key)}
   bySymbol[key]=len(m.Symbols)
   m.Symbols=append(m.Symbols,key)
  }
 }
 add:=func(caller,callee,position string,cross bool)error{
  _,a:=bySymbol[caller];_,b:=bySymbol[callee]
  if !a||!b{return fmt.Errorf("internal error: unresolved local edge %s to %s",caller,callee)}
  m.Edges=append(m.Edges,ModuleEdge{
   Caller:caller,Callee:callee,Position:position,CrossPackage:cross,
  })
  return nil
 }
 for _,g:=range m.Packages{
  for _,call:=range g.Calls{
   if err:=add(g.Symbols[call.Caller].Qualified(),g.Symbols[call.Callee].Qualified(),call.Position,false);err!=nil{return nil,err}
  }
  for _,call:=range g.ImportedCalls{
   if _,found:=bySymbol[call.Callee];found{
    if err:=add(g.Symbols[call.Caller].Qualified(),call.Callee,call.Position,true);err!=nil{return nil,err}
   }else{m.UnlinkedImports++}
  }
 }
 n:=len(m.Symbols)
 m.Offsets=make([]uint32,n+1)
 for _,edge:=range m.Edges{m.Offsets[bySymbol[edge.Callee]+1]++}
 for i:=1;i<len(m.Offsets);i++{m.Offsets[i]+=m.Offsets[i-1]}
 m.Callers=make([]uint32,len(m.Edges))
 cursors:=append([]uint32(nil),m.Offsets[:n]...)
 for _,edge:=range m.Edges{
  target:=bySymbol[edge.Callee]
  m.Callers[cursors[target]]=uint32(bySymbol[edge.Caller])
  cursors[target]++
 }
 return m,nil
}

func (m *ModuleGraph) Impact(query string)(ModuleImpact,error){
 out:=ModuleImpact{Schema:"mbvm-module-impact-v1",
  DirectCallers:[]string{},TransitiveCandidates:[]string{},
  Warnings:[]string{
   "Only loaded local Go packages and statically resolved call sites are included.",
   "Interface dispatch, function values and reflection are not assigned to concrete callees.",
   "Graph reachability is not proof of compile-time or runtime breakage.",
  }}
 if m==nil{return out,fmt.Errorf("nil module graph")}
 out.Packages=len(m.Packages);out.Symbols=len(m.Symbols)
 out.ResolvedEdges=len(m.Edges);out.UnlinkedImports=m.UnlinkedImports
 found:=-1
 for i,name:=range m.Symbols{
  if name==query{found=i;break}
 }
 if found<0{return out,fmt.Errorf("no fully qualified symbol %q in analyzed packages",query)}
 out.Symbol=query
 n:=len(m.Symbols);seen:=make([]bool,n)
 queue:=make([]int,n);depths:=make([]int,n)
 queue[0]=found;seen[found]=true;head,tail:=0,1
 for head<tail{
  curr:=queue[head];depth:=depths[head];head++
  if depth>out.MaxDepth{out.MaxDepth=depth}
  for p:=m.Offsets[curr];p<m.Offsets[curr+1];p++{
   caller:=int(m.Callers[p]);if seen[caller]{continue}
   seen[caller]=true
   queue[tail]=caller;depths[tail]=depth+1;tail++
   if depth==0{out.DirectCallers=append(out.DirectCallers,m.Symbols[caller])}else{
    out.TransitiveCandidates=append(out.TransitiveCandidates,m.Symbols[caller])
   }
  }
 }
 sort.Strings(out.DirectCallers);sort.Strings(out.TransitiveCandidates)
 return out,nil
}

func (m *ModuleGraph) FindSymbol(query string)(*Graph,int,error){
 if m==nil{return nil,-1,fmt.Errorf("nil module graph")}
 for _,g:=range m.Packages{
  for i,s:=range g.Symbols{if s.Qualified()==query{return g,i,nil}}
 }
 return nil,-1,fmt.Errorf("symbol not found: %s",query)
}

// ModuleImportPaths returns only the packages known to make static calls
// to the requested target, optionally filtered by the caller's package.
func (m *ModuleGraph) DirectConsumerPackages(target string)[]string{
 unique:=map[string]bool{}
 for _,e:=range m.Edges{
  if e.Callee==target&&e.CrossPackage{
   for _,g:=range m.Packages{
    if strings.HasPrefix(e.Caller,g.PackagePath+"."){unique[g.PackagePath]=true;break}
   }
  }
 }
 result:=make([]string,0,len(unique))
 for pkg:=range unique{result=append(result,pkg)}
 sort.Strings(result)
 return result
}
