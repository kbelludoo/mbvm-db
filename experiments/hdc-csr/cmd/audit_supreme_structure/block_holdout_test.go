package main

import "testing"

func TestBlockHoldoutSyntheticGroupSeparation(t *testing.T) {
 for _,grouped:=range []bool{true,false}{
  data:=syntheticFixture(grouped)
  vectors,_,err:=load(data,10000)
  if err!=nil{t.Fatal(err)}
  report,err:=analyzeBlockHoldout(vectors,80)
  if err!=nil{t.Fatal(err)}
  t.Logf("grouped=%v crossblock Top1=%.2f%% nonduplicate=%.2f%% uniform baseline=%.2f%% margin=%.1f bits",
   grouped,100*report.CrossBlockTop1Accuracy,100*report.CrossBlockNonidenticalTop1Accuracy,
   100*report.RandomSameDomainBaseline,report.MeanSeparationMarginBits)
  if grouped{
   if report.CrossBlockTop1Accuracy!=1||report.CrossBlockNonidenticalTop1Accuracy!=1{
    t.Fatal("known clustered fixture was not retrieved across blocks")
   }
   if report.MinimumSeparationMarginBits<2000{
    t.Fatalf("unexpectedly small known-cluster margin: %d",report.MinimumSeparationMarginBits)
   }
  }else if report.CrossBlockTop1Accuracy>=.4{
   t.Fatal("independent random vectors spuriously classified as same-domain")
  }
 }
}

func TestBlockHoldoutInputValidation(t *testing.T) {
 if _,err:=analyzeBlockHoldout(make([][]byte,300),80);err==nil{
  t.Fatal("accepted wrong V4 vector count")
 }
 rows:=make([][]byte,10000)
 if _,err:=analyzeBlockHoldout(rows,0);err==nil{
  t.Fatal("accepted zero queries")
 }
 if _,err:=analyzeBlockHoldout(rows,2001);err==nil{
  t.Fatal("accepted excessive query count")
 }
}
