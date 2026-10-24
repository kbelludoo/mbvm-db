package main

import (
	"encoding/binary"
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
