package main

import (
	"encoding/binary"
	"math"
	"testing"
)

func fakeIndex(version uint32, vectors uint32) []byte {
	headerSize:=18
	if version==2{headerSize=64}
	raw:=make([]byte,headerSize+int(vectors)*bytesPerVector)
	copy(raw[:8],magic)
	binary.LittleEndian.PutUint32(raw[8:12],version)
	binary.LittleEndian.PutUint32(raw[12:16],vectors)
	binary.LittleEndian.PutUint16(raw[16:18],8192)
	return raw
}

func TestIndexAuditV1V2AndBitStatistics(t *testing.T) {
	for _,version:=range []uint32{1,2}{
		raw:=fakeIndex(version,3)
		offset:=18
		if version==2{offset=64}
		// v0 = all-zero; v1 = all-ones; v2 = exact copy of v0
		for i:=offset+bytesPerVector;i<offset+2*bytesPerVector;i++{raw[i]=0xff}
		report,err:=inspect(raw,100,3)
		if err!=nil{t.Fatalf("v%d: %v",version,err)}
		if report.Version!=version||report.HeaderBytes!=offset||report.VectorCount!=3 {
			t.Fatalf("v%d: incorrect header metadata: %+v",version,report)
		}
		if report.ExactDuplicateVectors!=1 {t.Fatalf("v%d: duplicate count %d",version,report.ExactDuplicateVectors)}
		if report.ConstantBitPositions!=0 {t.Fatalf("v%d: unexpected constant positions %d",version,report.ConstantBitPositions)}
		if report.MeanBitOneFraction < .33332||report.MeanBitOneFraction>.33334 {
			t.Fatalf("v%d: ones ratio %f",version,report.MeanBitOneFraction)
		}
		if report.MinNearestNonSelfHamming!=0||report.MaxNearestNonSelfHamming!=8192 {
			t.Fatalf("v%d: nearest extrema %d %d",version,
				report.MinNearestNonSelfHamming,report.MaxNearestNonSelfHamming)
		}
		wantGlobalMean:=2.0*8192.0/3.0
		if math.Abs(report.MeanAllDistinctPairHamming-wantGlobalMean)>1e-6 {
			t.Fatalf("v%d: exact global pair Hamming mean=%g, expected=%g",
				version, report.MeanAllDistinctPairHamming,wantGlobalMean)
		}
		const wantEntropy=0.9182958340544896 // H_2(1/3), bits per column
		if math.Abs(report.MeanMarginalBitEntropy-wantEntropy)>1e-12 {
			t.Fatalf("v%d: marginal entropy %g",version,report.MeanMarginalBitEntropy)
		}
		if report.Schema!="mbvm-index-audit-v2" {
			t.Fatalf("v%d: report schema %q",version,report.Schema)
		}
		if report.ExpectedFileBytes!=int64(len(raw)) || report.FileSHA256=="" {
			t.Fatalf("v%d: invalid size/hash",version)
		}
	}
}

func TestIndexAuditRejectBadHeadersAndLengths(t *testing.T) {
	base:=fakeIndex(1,2)
	cases:=map[string][]byte{
		"truncated header": base[:12],
		"truncated body": base[:len(base)-1],
		"wrong magic":append([]byte("NOTREAL!"),base[8:]...),
		"unsupported version":func()[]byte{v:=append([]byte{},base...);binary.LittleEndian.PutUint32(v[8:12],3);return v}(),
		"wrong dimensions":func()[]byte{v:=append([]byte{},base...);binary.LittleEndian.PutUint16(v[16:18],4096);return v}(),
	}
	for label,raw:=range cases{
		if _,err:=inspect(raw,100,1);err==nil{t.Fatalf("%s should be rejected",label)}
	}
	if _,err:=inspect(base,1,1);err==nil{
		t.Fatal("max-vectors safety bound not enforced")
	}
	v2:=fakeIndex(2,2)
	v2[18]=1
	if _,err:=inspect(v2,100,1);err==nil{
		t.Fatal("nonzero v2 header padding accepted")
	}
}

func TestExactPairHammingUnbiasedByNodeOrdering(t *testing.T) {
	// Eight pair combinations are fixed by the data, regardless of order.
	// A fixed offset can be manipulated by permuting nodes, while the
	// per-coordinate exact all-pairs mean cannot.
	raw:=fakeIndex(2,4)
	start:=64
	copy(raw[start+0*1024:start+1*1024],make([]byte,1024))
	for i:=start+1024;i<start+2048;i++ {raw[i]=0xff}
	for i:=start+2048;i<start+3072;i++ {raw[i]=0xaa}
	for i:=start+3072;i<start+4096;i++ {raw[i]=0x55}
	before,err:=inspect(raw,100,4)
	if err!=nil {t.Fatal(err)}
	// Swap vectors 1 and 2 to alter fixed-offset neighborhoods.
	tmp:=append([]byte{},raw[start+1024:start+2048]...)
	copy(raw[start+1024:start+2048],raw[start+2048:start+3072])
	copy(raw[start+2048:start+3072],tmp)
	after,err:=inspect(raw,100,4)
	if err!=nil {t.Fatal(err)}
	if math.Abs(before.MeanAllDistinctPairHamming-after.MeanAllDistinctPairHamming)>1e-8 {
		t.Fatalf("global Hamming population mean changed under permutation: %f vs %f",before.MeanAllDistinctPairHamming,after.MeanAllDistinctPairHamming)
	}
	if math.Abs(before.MeanAllDistinctPairHamming-8192.0*2/3)>1e-8 {
		t.Fatalf("unexpected population mean %f",before.MeanAllDistinctPairHamming)
	}
}
