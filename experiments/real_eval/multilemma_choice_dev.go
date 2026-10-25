package realeval

import (
 "errors"
 "fmt"
 "math/big"
 "sort"
 "strings"
 "time"
 erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

var errChoiceInapplicable=errors.New("inapplicable proof branch")

// Public CRT competing-path development control. Identical AND/OR planner,
// prerequisite semantics, proof composer and verifier in both retrieval arms.
type CRTChoiceLab struct {
 Base *CRTDepthLab
 Catalog []*erdosstraus.Rule
 Graph *erdosstraus.MathCSRGraph
 Tasks []CRTChoiceTask
 byID map[string]*erdosstraus.Rule
 orNodes map[string]bool
}
type CRTChoiceTask struct {
 ID string `json:"id"`
 GoalID string `json:"goal_id"`
 QueryN string `json:"query_n"`
 PlanIDs []string `json:"plan_ids"`
 BaseTask CRTDepthTask `json:"-"`
}
type CRTChoiceArm struct {
 Arm string `json:"arm"`
 SelectedPlanID string `json:"selected_plan_id"`
 TraceIDs []string `json:"trace_ids"`
 RetrievedIDs []string `json:"retrieved_ids"`
 VisitedNodes int `json:"visited_nodes"`
 RejectedAlternatives int `json:"rejected_alternatives"`
 ContextBytes int `json:"context_bytes"`
 RetrievalNanos int64 `json:"retrieval_nanoseconds"`
 Certificate *CRTDepthCertificate `json:"certificate,omitempty"`
 Certified bool `json:"certified"`
 Error string `json:"error,omitempty"`
}
type CRTChoicePair struct {
 TaskID string `json:"task_id"`
 Conventional CRTChoiceArm `json:"conventional"`
 CSR CRTChoiceArm `json:"csr"`
 SameContext bool `json:"same_context"`
 SameCertificate bool `json:"same_certificate"`
}

// GOAL(OR) -> SHORT(L1,L2,L3), LONG(MID,L3),
// BAD(NOISE1,L2,L3), OVERLOAD(MID,L3,NOISE2). MID -> L1,L2.
// The query instance satisfies the true premises, not the distractors.
// No plan stores the answer.
func NewCRTChoiceLab()(*CRTChoiceLab,error){
 base:=NewCRTDepthLab()
 lab:=&CRTChoiceLab{Base:base,Catalog:append([]*erdosstraus.Rule(nil),base.Catalog...),
  byID:make(map[string]*erdosstraus.Rule),orNodes:make(map[string]bool)}
 for id,r:=range base.byID{lab.byID[id]=r}
 for _,task:=range base.Tasks{
  leaves,e:=base.retrieveDepthIndex(task.GoalID);if e!=nil{return nil,e}
  cert,e:=base.composeDepth(task,leaves);if e!=nil{return nil,e}
  short,long,bad,over,goal:=task.ID+"-SHORT",task.ID+"-LONG",task.ID+"-BAD",task.ID+"-OVERLOAD",task.ID+"-CHOICE"
  plans:=[]struct{id string;deps []string}{
   {short,append([]string(nil),task.LeafIDs...)},
   {long,[]string{task.MidID,task.LeafIDs[2]}},
   {bad,[]string{task.NoiseIDs[0],task.LeafIDs[1],task.LeafIDs[2]}},
   {over,[]string{task.MidID,task.LeafIDs[2],task.NoiseIDs[1]}},
   {goal,[]string{short,long,bad,over}},
  }
  for _,p:=range plans{
   r:=&erdosstraus.Rule{ID:p.id,Name:"CRT development proof plan",Dependencies:p.deps,
    Conclusion:"No CRT residue or numeric solution stored in this node"}
   lab.Catalog=append(lab.Catalog,r);lab.byID[r.ID]=r
  }
  lab.orNodes[goal]=true
  lab.Tasks=append(lab.Tasks,CRTChoiceTask{ID:task.ID,GoalID:goal,QueryN:cert.Final.Residue,
   BaseTask:task,PlanIDs:[]string{short,long,bad,over}})
 }
 lab.Graph=erdosstraus.NewMathCSRGraph(lab.Catalog)
 return lab,nil
}

func (l *CRTChoiceLab) indexChildren(id string)([]string,error){
 r,ok:=l.byID[id];if !ok{return nil,fmt.Errorf("unknown index node %s",id)}
 return append([]string(nil),r.Dependencies...),nil
}
func (l *CRTChoiceLab) csrChildren(id string)([]string,error){
 idx,ok:=l.Graph.IDToIdx[id];if !ok{return nil,fmt.Errorf("unknown CSR node %s",id)}
 var deps []string
 for _,e:=range l.Graph.Neighbors(idx){
  if e.Rel!=erdosstraus.RelDependency{continue}
  if e.Target<0||e.Target>=len(l.Graph.RuleIDs){return nil,fmt.Errorf("invalid CSR edge target %d",e.Target)}
  deps=append(deps,l.Graph.RuleIDs[e.Target])
 }
 return deps,nil
}
type choiceState struct{
 nodes map[string]bool
 leaves map[string]*erdosstraus.Rule
}
func newChoiceState()choiceState{return choiceState{nodes:map[string]bool{},leaves:map[string]*erdosstraus.Rule{}}}
func choiceIDs[V any](m map[string]V)[]string{
 out:=make([]string,0,len(m));for id:=range m{out=append(out,id)};sort.Strings(out);return out
}
func choiceMerge(dst *choiceState,part choiceState){
 for id:=range part.nodes{dst.nodes[id]=true}
 for id,r:=range part.leaves{dst.leaves[id]=r}
}

// The common proof selector chooses the fewest DISTINCT proof nodes;
// tie-breaking is lexical, not dependent on adjacency order. All alternative
// plans are evaluated, and incompatible modular premises are rejected.
func (l *CRTChoiceLab) selectChoice(goal string,n *big.Int,
 children func(string)([]string,error),disabled map[string]bool,
)(choiceState,int,int,error){
 visited,rejected:=0,0
 var walk func(string,map[string]bool)(choiceState,error)
 walk=func(id string,active map[string]bool)(choiceState,error){
  visited++
  if disabled[id]{return choiceState{},fmt.Errorf("%w: disabled %s",errChoiceInapplicable,id)}
  if active[id]{return choiceState{},fmt.Errorf("cyclic proof path at %s",id)}
  r,ok:=l.byID[id];if !ok{return choiceState{},fmt.Errorf("unknown proof node %s",id)}
  state:=newChoiceState();state.nodes[id]=true
  if r.Modulus>0{
   if len(r.Residues)!=1||!r.Covers(n){return choiceState{},fmt.Errorf("%w: %s",errChoiceInapplicable,id)}
   state.leaves[id]=r;return state,nil
  }
  active[id]=true;defer delete(active,id)
  deps,err:=children(id);if err!=nil{return choiceState{},err}
  if len(deps)==0{return choiceState{},fmt.Errorf("empty proof node %s",id)}
  if l.orNodes[id]{
   var best choiceState
   bestScore:=int(^uint(0)>>1);bestKey:="";found:=false
   for _,dep:=range deps{
    candidate,e:=walk(dep,active)
    if e!=nil{
     if !errors.Is(e,errChoiceInapplicable){return choiceState{},e}
     rejected++;continue
    }
    candidate.nodes[id]=true
    key:=strings.Join(choiceIDs(candidate.nodes),",")
    score:=len(candidate.nodes)
    if !found||score<bestScore||(score==bestScore&&key<bestKey){
     best,bestScore,bestKey,found=candidate,score,key,true
    }
   }
   if !found{return choiceState{},fmt.Errorf("%w: no valid paths at %s",errChoiceInapplicable,id)}
   return best,nil
  }
  for _,dep:=range deps{
   part,e:=walk(dep,active);if e!=nil{return choiceState{},e}
   choiceMerge(&state,part)
  }
  return state,nil
 }
 state,err:=walk(goal,map[string]bool{})
 return state,visited,rejected,err
}

func (l *CRTChoiceLab) runChoiceArm(t CRTChoiceTask,name string,
 children func(string)([]string,error),
)(CRTChoiceArm,string){
 out:=CRTChoiceArm{Arm:name}
 n,ok:=new(big.Int).SetString(t.QueryN,10);if !ok{out.Error="bad query integer";return out,""}
 start:=time.Now()
 state,visited,rejected,e:=l.selectChoice(t.GoalID,n,children,nil)
 out.RetrievalNanos=time.Since(start).Nanoseconds()
 out.VisitedNodes=visited;out.RejectedAlternatives=rejected
 if e!=nil{out.Error=e.Error();return out,""}
 out.TraceIDs=choiceIDs(state.nodes);out.RetrievedIDs=choiceIDs(state.leaves)
 for _,id:=range t.PlanIDs{if state.nodes[id]{out.SelectedPlanID=id;break}}
 leaves:=make([]*erdosstraus.Rule,0,len(out.RetrievedIDs))
 for _,id:=range out.RetrievedIDs{leaves=append(leaves,state.leaves[id])}
 context:=formatCRTPremises(leaves);out.ContextBytes=len(context)
 out.Certificate,e=l.Base.composeDepth(t.BaseTask,leaves)
 if e==nil{
  R,okR:=new(big.Int).SetString(out.Certificate.Final.Residue,10)
  M,okM:=new(big.Int).SetString(out.Certificate.Final.Modulus,10)
  if !okR||!okM||M.Sign()<=0||new(big.Int).Mod(n,M).Cmp(R)!=0{
   e=fmt.Errorf("certified CRT class disagrees with query")
  }
 }
 if e!=nil{out.Error=e.Error()}else{out.Certified=true}
 return out,context
}
func(l *CRTChoiceLab)RunChoicePaired()([]CRTChoicePair,error){
 results:=make([]CRTChoicePair,0,len(l.Tasks))
 for _,t:=range l.Tasks{
  b,bc:=l.runChoiceArm(t,"B: indexed graph expansion",l.indexChildren)
  c,cc:=l.runChoiceArm(t,"C: CSR graph expansion",l.csrChildren)
  row:=CRTChoicePair{TaskID:t.ID,Conventional:b,CSR:c,
   SameContext:bc==cc,SameCertificate:sameDepthCertificates(b.Certificate,c.Certificate)}
  results=append(results,row)
  if !b.Certified||!c.Certified||!row.SameContext||!row.SameCertificate||b.SelectedPlanID!=c.SelectedPlanID{
   return results,fmt.Errorf("competing-path parity failed %s: B=%s C=%s",t.ID,b.Error,c.Error)
  }
 }
 return results,nil
}
