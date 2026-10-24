package hdccsr

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeProjectionKnownSignBits(t *testing.T) {
	p:=&NativeProjection8K{Weights:make([]float32,NativeProjectionInputDim*NativeProjectionOutputBits)}
	x:=make([]float32,NativeProjectionInputDim)
	x[0]=2
	x[1]=-1
	p.Weights[0]=1                        // positive => 1
	p.Weights[NativeProjectionInputDim]=-1 // negative => 0
	p.Weights[2*NativeProjectionInputDim+1]=2 // negative => 0
	p.Weights[3*NativeProjectionInputDim+1]=-2 // positive => 1
	got,err:=p.ProjectNormalizedMiniLM384(x)
	if err!=nil{t.Fatal(err)}
	if got[0]&0xf != 9 {t.Fatalf("want first four bit values 1001 (LSB-first); got %04b",got[0]&0xf)}
	// Zero logits are mapped to 1, precisely as Python exporter (z>=0).
	if got[0]>>4 != ^uint64(0)>>4 {t.Fatalf("zero-logit sign mismatch")}
	if got[127]!=^uint64(0) {t.Fatal("last projection word has zero-logit bit mismatch")}
	x[22]=float32(math.NaN())
	if _,err:=p.ProjectNormalizedMiniLM384(x);err==nil{t.Fatal("NaN embedding accepted")}
	if _,err:=p.ProjectNormalizedMiniLM384([]float32{1,2});err==nil{t.Fatal("wrong embedding dimension accepted")}
}

func writeProjectionV1TestFile(t *testing.T,path string) {
	t.Helper()
	f,err:=os.Create(path)
	if err!=nil{t.Fatal(err)}
	defer f.Close()
	var header [NativeProjectionHeaderBytes]byte
	copy(header[:8],NativeProjectionMagic)
	binary.LittleEndian.PutUint32(header[8:12],NativeProjectionVersion)
	binary.LittleEndian.PutUint32(header[12:16],NativeProjectionInputDim)
	binary.LittleEndian.PutUint32(header[16:20],NativeProjectionOutputBits)
	binary.LittleEndian.PutUint32(header[20:24],NativeProjectionFloatFormat)
	for i:=56;i<88;i++ {header[i]=0xab}
	if _,err=f.Write(header[:]);err!=nil{t.Fatal(err)}
	h:=sha256.New()
	buf:=make([]byte,4096) // all 0.0 weights
	for left:=NativeProjectionWeightBytes;left>0; {
		n:=len(buf)
		if left<n{n=left}
		if _,err=f.Write(buf[:n]);err!=nil{t.Fatal(err)}
		_,_=h.Write(buf[:n])
		left-=n
	}
	copy(header[24:56],h.Sum(nil))
	if _,err=f.WriteAt(header[:],0);err!=nil{t.Fatal(err)}
}

func TestNativeProjectionFileValidation(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"projection.v1.bin")
	writeProjectionV1TestFile(t,path)
	p,err:=LoadNativeProjection8K(path)
	if err!=nil{t.Fatal(err)}
	if len(p.Weights)!=8192*384 {t.Fatal("weight count mismatch")}
	if len(p.SourceDigest)!=64 || p.SourceDigest[:4]!="abab"{t.Fatal("wrong source digest")}
	x:=make([]float32,384)
	x[0]=1
	got,err:=p.ProjectNormalizedMiniLM384(x)
	if err!=nil{t.Fatal(err)}
	for _,word:=range got {
		if word!=^uint64(0){t.Fatal("zero weight must emit all one bits")}
	}
	f,err:=os.OpenFile(path,os.O_RDWR,0600)
	if err!=nil{t.Fatal(err)}
	if _,err=f.WriteAt([]byte{1},NativeProjectionHeaderBytes);err!=nil{t.Fatal(err)}
	if err=f.Close();err!=nil{t.Fatal(err)}
	if _,err:=LoadNativeProjection8K(path);err==nil{
		t.Fatal("weight corruption was not rejected by SHA-256")
	}
}

func TestNativeProjectionMiniLMFeatureInput(t *testing.T){
	file:=filepath.Join(t.TempDir(),"one.f32le")
	data:=make([]byte,384*4)
	binary.LittleEndian.PutUint32(data[0:4],math.Float32bits(1.5))
	if err:=os.WriteFile(file,data,0600);err!=nil{t.Fatal(err)}
	vec,err:=LoadMiniLM384Float32LE(file)
	if err!=nil{t.Fatal(err)}
	if len(vec)!=384||vec[0]!=1.5||vec[383]!=0{t.Fatal("feature parse mismatch")}
}
