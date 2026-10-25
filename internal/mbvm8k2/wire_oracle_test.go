package mbvm8k2

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

// This external wire-format oracle deliberately does not call Decode,
// sectionBytes, or the indexed reader; it interprets offsets directly
// from the emitted bytes with the standard library.
func TestV2IndependentWireOracle(t *testing.T) {
	raw:=encodeFixture(t,Snapshot{
		Symbols:[]Symbol{{Qualified:"example.Z",Name:"Z"},{Qualified:"example.A",Name:"A"}},
		Edges:[]Edge{{Caller:0,Callee:1,Position:"z.go:2"},
			{Caller:0,Callee:1,Position:"z.go:2"}},
	})
	get16:=func(p int)uint16{return binary.LittleEndian.Uint16(raw[p:p+2])}
	get32:=func(p int)uint32{return binary.LittleEndian.Uint32(raw[p:p+4])}
	get64:=func(p int)uint64{return binary.LittleEndian.Uint64(raw[p:p+8])}
	if string(raw[:7])!="MBVM8K2" || raw[7]!=0 ||
		get16(8)!=2 || get16(10)!=0 || get32(12)!=128 ||
		get32(20)!=6 || get64(24)!=128 ||
		get64(32)!=uint64(len(raw)) || get64(40)!=2 {
		t.Fatal("header does not match independently specified v2 offsets")
	}
	clear:=bytes.Clone(raw)
	for i:=48;i<80;i++{clear[i]=0}
	d:=sha256.Sum256(clear)
	if !bytes.Equal(raw[48:80],d[:]){t.Fatal("whole-file digest differs")}
	entries:=map[uint32][3]uint64{}
	for i:=0;i<6;i++{
		p:=128+48*i
		id,offset,length:=get32(p),get64(p+8),get64(p+16)
		if id!=uint32(i+1) || offset%8!=0 || offset+length>uint64(len(raw)) {
			t.Fatalf("wrong directory entry %d",i)
		}
		crc:=crc32.Checksum(raw[offset:offset+length],crc32.MakeTable(crc32.Castagnoli))
		if crc!=get32(p+40){t.Fatalf("bad CRC in section %d",i+1)}
		entries[id]=[3]uint64{offset,length,get64(p+24)}
	}
	// Lexical order A/Z means caller 1 -> callee 0, twice, preserving calls.
	o:=int(entries[3][0])
	if get32(o)!=0||get32(o+4)!=0||get32(o+8)!=2{
		t.Fatalf("unexpected independent forward offsets: %v",raw[o:o+12])
	}
	o=int(entries[4][0])
	for i:=0;i<2;i++{
		at:=o+16*i
		if get32(at)!=1 || get32(at+4)!=0 {
			t.Fatalf("incorrect independent outgoing edge %d",i)
		}
	}
	o=int(entries[5][0])
	if get32(o)!=0||get32(o+4)!=2||get32(o+8)!=2{
		t.Fatal("incorrect independently decoded reverse row offsets")
	}
	o=int(entries[6][0])
	if get32(o)!=0||get32(o+4)!=1{
		t.Fatal("reverse CSR is not [0,1]")
	}
}

func TestV2CanonicalPermutationAndReadLimits(t *testing.T) {
	first:=Snapshot{
		Symbols:[]Symbol{{Qualified:"z.Z"},{Qualified:"a.A"}},
		Edges:[]Edge{{Caller:0,Callee:1,Position:"line2"},
			{Caller:1,Callee:0,Position:"line1"}},
	}
	second:=Snapshot{
		Symbols:[]Symbol{{Qualified:"a.A"},{Qualified:"z.Z"}},
		Edges:[]Edge{{Caller:0,Callee:1,Position:"line1"},
			{Caller:1,Callee:0,Position:"line2"}},
	}
	a,b:=encodeFixture(t,first),encodeFixture(t,second)
	if !bytes.Equal(a,b) {t.Fatal("canonical wire differs with input symbol order")}
	for _,n:=range []int64{-1,0,int64(len(a))+100}{
		if _,err:=Decode(bytes.NewReader(a),n,Limits{MaxFileBytes:1024});err==nil{
			t.Fatalf("decoder accepted incorrect physical length %d",n)
		}
	}
}
