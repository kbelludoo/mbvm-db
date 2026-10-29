package realeval

import (
 "errors"
 "math/big"
 "strings"
 "testing"
)

func TestCRTChoiceCompetingPathsParity(t *testing.T){
 lab,err:=NewCRTChoiceLab();if err!=nil{t.Fatal(err)}
 if len(lab.Tasks)!=12{t.Fatalf("want 12 choice tasks, got %d",len(lab.Tasks))}
 if len(lab.Catalog)!=12*12{t.Fatalf("want 144 nodes including distractors and competing plans, got %d",len(lab.Catalog))}
 rows,err:=lab.RunChoicePaired();if err!=nil{t.Fatal(err)}
 if len(rows)!=12{t.Fatalf("want 12 results, got %d",len(rows))}
 for i,row:=range rows{
  task:=lab.Tasks[i]
  for _,arm:=range []CRTChoiceArm{row.Conventional,row.CSR}{
   if !arm.Certified{t.Fatalf("%s: certificate rejected: %s",task.ID,arm.Error)}
   if arm.SelectedPlanID!=task.PlanIDs[0]{t.Fatalf("%s: minimal SHORT plan not selected: %s",task.ID,arm.SelectedPlanID)}
   if arm.RejectedAlternatives!=2{t.Fatalf("%s: want 2 incompatible branches rejected, got %d",task.ID,arm.RejectedAlternatives)}
   if len(arm.RetrievedIDs)!=3{t.Fatalf("%s: got %d premises, want 3",task.ID,len(arm.RetrievedIDs))}
   if len(arm.TraceIDs)!=5{t.Fatalf("%s: expected 5 nodes in minimal trace, got %d",task.ID,len(arm.TraceIDs))}
   if arm.ContextBytes==0 {t.Fatalf("%s: no context",task.ID)}
   for _,noise:=range task.BaseTask.NoiseIDs {
    if strings.Contains(strings.Join(arm.TraceIDs,","),noise){t.Fatalf("%s: incompatible noise %s selected",task.ID,noise)}
   }
  }
  if !row.SameContext||!row.SameCertificate||row.Conventional.SelectedPlanID!=row.CSR.SelectedPlanID{
   t.Fatalf("%s: conventional and CSR diverged",task.ID)
  }
 }
}

func TestCRTChoiceFallsBackToLongerValidProof(t *testing.T){
 lab,err:=NewCRTChoiceLab();if err!=nil{t.Fatal(err)}
 for _,task:=range lab.Tasks {
  n,ok:=new(big.Int).SetString(task.QueryN,10)
  if !ok{t.Fatalf("%s: invalid query n",task.ID)}
  disabled:=map[string]bool{task.PlanIDs[0]:true}
  for _,backend:=range []struct{
   name string
   next func(string)([]string,error)
  }{{"index",lab.indexChildren},{"CSR",lab.csrChildren}}{
   state,_,rejected,e:=lab.selectChoice(task.GoalID,n,backend.next,disabled)
   if e!=nil{t.Fatalf("%s %s: failed fallback: %v",task.ID,backend.name,e)}
   if !state.nodes[task.PlanIDs[1]]||state.nodes[task.PlanIDs[0]]{
    t.Fatalf("%s %s: LONG plan not selected after SHORT disabled",task.ID,backend.name)
   }
   if len(state.nodes)!=6||len(state.leaves)!=3{
    t.Fatalf("%s %s: wrong fallback trace size: nodes=%d leaves=%d",task.ID,backend.name,len(state.nodes),len(state.leaves))
   }
   if rejected!=3{t.Fatalf("%s %s: want 3 rejected alternatives, got %d",task.ID,backend.name,rejected)}
  }
 }
}

func TestCRTChoiceRejectsUnsatisfiableAlternatives(t *testing.T){
 lab,err:=NewCRTChoiceLab();if err!=nil{t.Fatal(err)}
 task:=lab.Tasks[0]
 n,_:=new(big.Int).SetString(task.QueryN,10)
 disabled:=map[string]bool{task.PlanIDs[0]:true,task.PlanIDs[1]:true}
 for _,next:=range []func(string)([]string,error){lab.indexChildren,lab.csrChildren}{
  _,_,_,e:=lab.selectChoice(task.GoalID,n,next,disabled)
  if !errors.Is(e,errChoiceInapplicable){t.Fatalf("two bad branches should not be certified: %v",e)}
 }
}

// This verifies planner behavior, not proof discovery. The objective and its
// dependencies were supplied to both controls and are PUBLIC fixtures.
