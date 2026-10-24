package main

import (
 "encoding/binary"
 "math"
 "math/rand"
 "testing"
)

func syntheticFixture(grouped bool)[]byte{
 const count=10000
 out:=make([]byte,64+count*vectorBytes)
 copy(out[:8],"MBVM8K1\x00")
 binary.LittleEndian.PutUint32(out[8:12],2)
 binary.LittleEndian.PutUint32(out[12:16],count)
 binary.LittleEndian.PutUint16(out[16:18],vectorBits)
 rng:=rand.New(rand.NewSource(3))
 var bases [8][vectorBytes]byte
 for group:=range bases{_,_=rng.Read(bases[group][:])}
 for i:=0;i<count;i++{
  row:=out[64+i*vectorBytes:64+(i+1)*vectorBytes]
  if !grouped{_,_=rng.Read(row);continue}
  copy(row,bases[inferGroup(i)][:])
  for b:=0;b<64;b++{pos:=rng.Intn(vectorBits);row[pos/8]^=1<<uint(pos%8)}
 }
 return out
}
func TestStructureAuditRejectsMalformedBinary(t *testing.T){
 blob:=syntheticFixture(false)
 if _,_,err:=load(blob,10000);err!=nil{t.Fatal(err)}
 if _,_,err:=load(blob[:len(blob)-1],10000);err==nil{t.Fatal("accepted truncated input")}
 blob[18]=1
 if _,_,err:=load(blob,10000);err==nil{t.Fatal("accepted v2 padding corruption")}
}
func TestStructureAuditSyntheticVsIndependent(t *testing.T){
 for _,grouped:=range []bool{true,false}{
  data:=syntheticFixture(grouped)
  rows,version,err:=load(data,10000)
  if err!=nil{t.Fatal(err)}
  r:=analyze(rows,version,2026,64,96,8,true)
  if r.GroupWithin==nil||r.GroupAcross==nil||r.NearestSameGroupRate==nil{
   t.Fatal("incomplete group analysis")
  }
  if grouped{
   if *r.MeanDistanceGap<2000{t.Fatalf("expected strong domain gap, got %v",*r.MeanDistanceGap)}
   if *r.NearestSameGroupRate!=1{t.Fatalf("nearest should be same group: %g",*r.NearestSameGroupRate)}
  }else{
   if math.Abs(*r.MeanDistanceGap)>450{t.Fatalf("random vectors appear structured by labels: %g",*r.MeanDistanceGap)}
  }
  t.Logf("grouped=%v within=%f across=%f mean_abs_phi=%f same_nearest=%f",
   grouped,r.GroupWithin.Mean,r.GroupAcross.Mean,r.MeanAbsBitPhi,*r.NearestSameGroupRate)
 }
}
func TestStructureAuditUnlabeledMode(t *testing.T){
 data:=syntheticFixture(false)
 rows,version,err:=load(data,10000);if err!=nil{t.Fatal(err)}
 r:=analyze(rows,version,42,16,16,1,false)
 if r.GroupWithin!=nil||r.NearestSameGroupRate!=nil||r.Layout!="none"{
  t.Fatal("inferred labels appeared without explicit opt-in")
 }
}
