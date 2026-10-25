package mbvm8k2

import (
	"bytes"
	"errors"
	"hash/crc32"
	"testing"
)

func encodeFixture(t *testing.T, s Snapshot) []byte {
	t.Helper()
	var b bytes.Buffer
	if err:=Encode(&b,s);err!=nil{t.Fatal(err)}
	return b.Bytes()
}
func decodeFixture(t *testing.T, raw []byte) Snapshot {
	t.Helper()
	s,err:=Decode(bytes.NewReader(raw),int64(len(raw)),Limits{})
	if err!=nil{t.Fatal(err)}
	return s
}

func TestV2EmptyAndDeterministicRoundtrip(t *testing.T) {
	empty:=encodeFixture(t,Snapshot{})
	got:=decodeFixture(t,empty)
	if len(got.Symbols)!=0 || len(got.Edges)!=0 {t.Fatalf("invalid empty snapshot: %+v",got)}
	if !bytes.Equal(empty[:8],magic[:]) || len(empty)<headerSize+6*directoryEntrySize {
		t.Fatal("wrong v2 header")
	}
	for i:=0;i<10;i++ {
		if b:=encodeFixture(t,Snapshot{});!bytes.Equal(b,empty){
			t.Fatal("empty container bytes changed")
		}
	}
	input:=Snapshot{
		Symbols:[]Symbol{
			{Qualified:"pkg.Z",Name:"Z",Package:"pkg",Kind:"func"},
			{Qualified:"pkg.A",Name:"A",Package:"pkg",Kind:"func"},
			{Qualified:"pkg.M",Name:"M",Package:"pkg",Kind:"method",Receiver:"Z"},
		},
		Edges:[]Edge{
			{Caller:0,Callee:1,Position:"a.go:10",CrossPackage:false},
			{Caller:2,Callee:1,Position:"a.go:11",CrossPackage:true},
			{Caller:0,Callee:1,Position:"a.go:10",CrossPackage:false},
			{Caller:1,Callee:2,Position:"a.go:9",CrossPackage:false},
		},
	}
	raw:=encodeFixture(t,input)
	snapshot:=decodeFixture(t,raw)
	if len(snapshot.Symbols)!=3 || len(snapshot.Edges)!=4 ||
		snapshot.Symbols[0].Qualified!="pkg.A" ||
		snapshot.Symbols[1].Qualified!="pkg.M" ||
		snapshot.Symbols[2].Qualified!="pkg.Z" {
		t.Fatalf("unstable symbols or lost edges: %+v",snapshot)
	}
	// Duplicate call-site edges are intentional and must not be folded.
	duplicates := 0
	for _,e := range snapshot.Edges {
		if e.Caller==2 && e.Callee==0 && e.Position=="a.go:10" && !e.CrossPackage {
			duplicates++
		}
	}
	if duplicates!=2 {
		t.Fatalf("wanted exactly two duplicate call-site edges, got %d: %+v",duplicates,snapshot.Edges)
	}
	for i:=0;i<20;i++ {
		if b:=encodeFixture(t,input);!bytes.Equal(b,raw){t.Fatal("nondeterministic v2 encoding")}
	}
	roundtrip:=encodeFixture(t,snapshot)
	if !bytes.Equal(roundtrip,raw){t.Fatal("decode->encode changed canonical bytes")}
}

func TestV2BytecodeOptionalAndInvalidInputs(t *testing.T) {
	s:=Snapshot{Symbols:[]Symbol{{Qualified:"pkg.X",Name:"X",BytecodeOffset:1,BytecodeLength:2}},
		Bytecode:[]byte{1,2,3,4}}
	if got:=decodeFixture(t,encodeFixture(t,s));!bytes.Equal(got.Bytecode,s.Bytecode) {
		t.Fatal("optional bytecode changed")
	}
	for _,bad:=range []Snapshot{
		{Symbols:[]Symbol{{Qualified:"pkg.X"},{Qualified:"pkg.X"}}},
		{Symbols:[]Symbol{{Qualified:""}}},
		{Symbols:[]Symbol{{Qualified:"bad\x00name"}}},
		{Symbols:[]Symbol{{Qualified:"bad\xff"}}},
		{Symbols:[]Symbol{{Qualified:"pkg.X"}},Edges:[]Edge{{Caller:0,Callee:1}}},
		{Symbols:[]Symbol{{Qualified:"pkg.X",BytecodeOffset:3,BytecodeLength:5}},Bytecode:[]byte{1,2}},
	} {
		if err:=Encode(&bytes.Buffer{},bad);!errors.Is(err,ErrInvalidFormat) {
			t.Fatalf("expected invalid input error: %+v got %v",bad,err)
		}
	}
}

func rewriteDigest(raw []byte) {
	for i:=48;i<80;i++{raw[i]=0}
	d:=fullFileDigest(raw)
	copy(raw[48:80],d[:])
}
func rewriteSectionCRC(raw []byte, id uint32) {
	for i:=uint32(0);i<le.Uint32(raw[20:]);i++ {
		p:=headerSize+int(i)*directoryEntrySize
		if le.Uint32(raw[p:])!=id {continue}
		off,size:=le.Uint64(raw[p+8:]),le.Uint64(raw[p+16:])
		le.PutUint32(raw[p+40:],crc32.Checksum(raw[off:off+size],castagnoli))
		rewriteDigest(raw)
		return
	}
}

func TestV2RejectsCorruptionEvenWithRecomputedHashes(t *testing.T) {
	s:=Snapshot{Symbols:[]Symbol{{Qualified:"pkg.A"},{Qualified:"pkg.B"}},
		Edges:[]Edge{{Caller:0,Callee:1,Position:"main.go:3"}}}
	original:=encodeFixture(t,s)
	cases:=[]struct{
		name string
		edit func([]byte)
	}{
		{"magic",func(b []byte){b[0]='X'}},
		{"version",func(b []byte){le.PutUint16(b[8:],3)}},
		{"reserved",func(b []byte){b[112]=1}},
		{"section crc",func(b []byte){b[len(b)-1]^=1}},
		{"unknown required",func(b []byte){le.PutUint32(b[headerSize:],100);rewriteDigest(b)}},
		{"bad alignment",func(b []byte){le.PutUint64(b[headerSize+8:],129);rewriteDigest(b)}},
		{"bad file length",func(b []byte){le.PutUint64(b[32:],99);rewriteDigest(b)}},
		{"invalid symbol string id",func(b []byte){
			p:=headerSize+directoryEntrySize
			off:=le.Uint64(b[p+8:])
			le.PutUint32(b[off:],0xffffffff)
			rewriteSectionCRC(b,sectionSymbols)
		}},
		{"invalid CSR offset",func(b []byte){
			p:=headerSize+2*directoryEntrySize
			off:=le.Uint64(b[p+8:])
			le.PutUint32(b[off:],1)
			rewriteSectionCRC(b,sectionOutOffset)
		}},
		{"duplicate reverse index",func(b []byte){
			p:=headerSize+5*directoryEntrySize
			off:=le.Uint64(b[p+8:])
			le.PutUint32(b[off:],42)
			rewriteSectionCRC(b,sectionInEdges)
		}},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T){
			b:=bytes.Clone(original)
			tc.edit(b)
			if _,err:=Decode(bytes.NewReader(b),int64(len(b)),Limits{});err==nil {
				t.Fatal("corruption accepted")
			}
		})
	}
	for length:=0;length<len(original);length++ {
		b:=original[:length]
		if _,err:=Decode(bytes.NewReader(b),int64(len(b)),Limits{});err==nil {
			t.Fatalf("truncated file accepted at byte %d",length)
		}
	}
	legacy:=[]byte{'M','B','V','M',1,0}
	if _,err:=Decode(bytes.NewReader(legacy),int64(len(legacy)),Limits{});err==nil {
		t.Fatal("v1 format incorrectly accepted by v2 reader")
	}
	if _,err:=Decode(bytes.NewReader(original),int64(len(original)),Limits{MaxFileBytes:128});err==nil {
		// Limits default caps at 256 MiB; custom small limits should be
		// enforced (reader currently adjusts custom >maximum only).
		t.Fatal("ignored caller file-size limit")
	}
}
