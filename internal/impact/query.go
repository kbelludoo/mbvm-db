package impact

import (
 "fmt"
 "path"
 "sort"
 "strings"
)

func (s Symbol) Qualified()string{
 if s.Receiver!=""{return s.PackagePath+".("+s.Receiver+")."+s.Name}
 return s.PackagePath+"."+s.Name
}

func (g *Graph) Select(query string)(int,error){
 if g==nil{return -1,fmt.Errorf("nil graph")}
 query=strings.TrimSpace(query)
 if query==""{return -1,fmt.Errorf("empty symbol query")}
 matches:=make([]int,0,2)
 for i,s:=range g.Symbols{
  q:=s.Qualified()
  local:=s.Name
  if s.Receiver!=""{local="("+s.Receiver+")."+s.Name}
  if query==q || query==local || query==s.Name ||
   query==path.Base(s.PackagePath)+"."+local ||
   query==path.Base(s.PackagePath)+"."+s.Name {
   matches=append(matches,i)
  }
 }
 if len(matches)==1{return matches[0],nil}
 if len(matches)==0{return -1,fmt.Errorf("symbol %q not found in package %s",query,g.PackagePath)}
 candidates:=make([]string,0,len(matches))
 for _,id:=range matches{candidates=append(candidates,g.Symbols[id].Qualified())}
 sort.Strings(candidates)
 return -1,fmt.Errorf("ambiguous symbol %q; choose a fully qualified name: %s",query,strings.Join(candidates,", "))
}

type Reach struct {
 Direct []int
 Transitive []int
 MaxDepth int
}

// BFS uses caller-allocated scratch for zero-heap cached queries.
// A node reached by multiple paths is attributed to its shortest path.
// Scratch must not be shared concurrently.
type Scratch struct {
 seen []uint32
 queue []uint32
 depths []int
 stamp uint32
}
func NewScratch(n int)*Scratch{return &Scratch{
 seen:make([]uint32,n),queue:make([]uint32,n),depths:make([]int,n),
}}
func (g *Graph) VisitCallers(target int,scratch *Scratch,visit func(id,depth int))(int,error){
 n:=len(g.Symbols)
 if target<0||target>=n||scratch==nil||
  len(scratch.queue)<n||len(scratch.seen)<n||len(scratch.depths)<n||
  len(g.Offsets)!=n+1{
  return 0,fmt.Errorf("invalid reverse graph query or scratch")
 }
 if scratch.stamp==^uint32(0){
  clear(scratch.seen);scratch.stamp=0
 }
 scratch.stamp++
 stamp:=scratch.stamp
 scratch.seen[target]=stamp
 scratch.queue[0]=uint32(target)
 scratch.depths[0]=0
 head,tail,maxDepth:=0,1,0
 for head<tail{
  id:=int(scratch.queue[head]);depth:=scratch.depths[head];head++
  if depth>maxDepth{maxDepth=depth}
  for j:=g.Offsets[id];j<g.Offsets[id+1];j++{
   caller:=int(g.Callers[j])
   if scratch.seen[caller]==stamp{continue}
   scratch.seen[caller]=stamp
   scratch.queue[tail]=uint32(caller)
   scratch.depths[tail]=depth+1
   tail++
   if visit!=nil{visit(caller,depth+1)}
  }
 }
 return maxDepth,nil
}
func (g *Graph) Reachable(target int)(Reach,error){
 out:=Reach{}
 _,err:=g.VisitCallers(target,NewScratch(len(g.Symbols)),func(id,depth int){
  if depth==1{out.Direct=append(out.Direct,id)}else{out.Transitive=append(out.Transitive,id)}
 })
 if err!=nil{return out,err}
 // Compute the maximum observed depth, including cycle-safe routing.
 // Caller lists remain stable and sorted by qualified symbol identity.
 scratch:=NewScratch(len(g.Symbols))
 out.MaxDepth,err=g.VisitCallers(target,scratch,nil)
 if err!=nil{return out,err}
 sort.Slice(out.Direct,func(i,j int)bool{return g.Symbols[out.Direct[i]].Qualified()<g.Symbols[out.Direct[j]].Qualified()})
 sort.Slice(out.Transitive,func(i,j int)bool{return g.Symbols[out.Transitive[i]].Qualified()<g.Symbols[out.Transitive[j]].Qualified()})
 return out,nil
}
