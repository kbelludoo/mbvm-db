package hdccsr

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	NativeProjectionMagic = "MBVMPJ1\x00"
	NativeProjectionHeaderBytes = 96
	NativeProjectionVersion = 1
	NativeProjectionInputDim = 384
	NativeProjectionOutputBits = 8192
	NativeProjectionFloatFormat = 1 // IEEE754 float32 little-endian
	NativeProjectionWeightBytes = NativeProjectionInputDim * NativeProjectionOutputBits * 4
)

// NativeProjection8K is only the 384D->8192 binary projection, NOT the
// preceding MiniLM text encoder. For raw Go source, callers must supply
// the SAME normalized MiniLM embeddings used to train this projection.
type NativeProjection8K struct {
	SourceDigest string // hex SHA-256 of the AST corpus used for training
	Weights []float32   // row-major [8192][384], read-only after loading
}

// LoadNativeProjection8K loads versioned, SHA-256 authenticated weights.
// The weight-file converter is an OFFLINE PyTorch tool; inference is Go-only.
func LoadNativeProjection8K(path string) (*NativeProjection8K,error) {
	f,err:=os.Open(path)
	if err!=nil {return nil,err}
	defer f.Close()
	st,err:=f.Stat()
	if err!=nil{return nil,err}
	expectedSize:=int64(NativeProjectionHeaderBytes+NativeProjectionWeightBytes)
	if st.Size()!=expectedSize {return nil,fmt.Errorf("projection file size %d, want %d",st.Size(),expectedSize)}
	var header [NativeProjectionHeaderBytes]byte
	if _,err=io.ReadFull(f,header[:]);err!=nil{return nil,err}
	if string(header[:8])!=NativeProjectionMagic ||
		binary.LittleEndian.Uint32(header[8:12])!=NativeProjectionVersion ||
		binary.LittleEndian.Uint32(header[12:16])!=NativeProjectionInputDim ||
		binary.LittleEndian.Uint32(header[16:20])!=NativeProjectionOutputBits ||
		binary.LittleEndian.Uint32(header[20:24])!=NativeProjectionFloatFormat {
		return nil,fmt.Errorf("projection header invalid or unsupported")
	}
	// 88..95 are reserved and MUST be zero for v1.
	for _,b:=range header[88:96]{
		if b!=0 {return nil,fmt.Errorf("unsupported nonzero v1 header reserved bytes")}
	}
	weights:=make([]float32,NativeProjectionInputDim*NativeProjectionOutputBits)
	h:=sha256.New()
	var buf [4096]byte
	for offset:=0;offset<len(weights);offset+=len(buf)/4 {
		n:=len(buf)/4
		if n>len(weights)-offset {n=len(weights)-offset}
		chunk:=buf[:n*4]
		if _,err=io.ReadFull(f,chunk);err!=nil{return nil,fmt.Errorf("truncated projection at weight %d: %w",offset,err)}
		_,_=h.Write(chunk)
		for i:=0;i<n;i++{
			weight:=math.Float32frombits(binary.LittleEndian.Uint32(chunk[i*4:]))
			if math.IsNaN(float64(weight)) || math.IsInf(float64(weight),0) {
				return nil,fmt.Errorf("non-finite projection weight %d",offset+i)
			}
			weights[offset+i]=weight
		}
	}
	actual:=h.Sum(nil)
	if string(actual)!=string(header[24:56]){return nil,fmt.Errorf("projection weight SHA-256 mismatch")}
	return &NativeProjection8K{SourceDigest:hex.EncodeToString(header[56:88]),Weights:weights},nil
}

// ProjectNormalizedMiniLM384 computes z = W*x and packs sign(z) into an
// 8192-bit Hypervector, with bit j at word j/64, bit offset j%64.
// PyTorch export likewise uses (z>=0), little-endian packed bits.
// Floating-point summation may differ in last bits around z=0 between
// backends; validate against the reference on real embeddings before claiming
// byte-for-byte parity.
func (p *NativeProjection8K) ProjectNormalizedMiniLM384(x []float32)(Hypervector,error){
	var result Hypervector
	if p==nil || len(p.Weights)!=NativeProjectionInputDim*NativeProjectionOutputBits{
		return result,fmt.Errorf("native projection is not initialized")
	}
	if len(x)!=NativeProjectionInputDim{
		return result,fmt.Errorf("input requires %d normalized MiniLM dimensions; got %d",NativeProjectionInputDim,len(x))
	}
	for i,feature:=range x{
		if math.IsNaN(float64(feature))||math.IsInf(float64(feature),0){
			return result,fmt.Errorf("non-finite embedding dimension %d",i)
		}
	}
	for bit:=0;bit<NativeProjectionOutputBits;bit++{
		base:=bit*NativeProjectionInputDim
		var z float32
		for j,feature:=range x {
			z+=p.Weights[base+j]*feature
		}
		if z>=0 {
			result[bit/64]|=uint64(1)<<uint(bit%64)
		}
	}
	return result,nil
}

// LoadMiniLM384Float32LE expects exactly 1536 bytes of normalized 384D
// float32 embeddings. Producing those embeddings is NOT covered by this
// lightweight Go projection loader.
func LoadMiniLM384Float32LE(path string)([]float32,error){
	raw,err:=os.ReadFile(path)
	if err!=nil{return nil,err}
	if len(raw)!=NativeProjectionInputDim*4{
		return nil,fmt.Errorf("MiniLM feature file needs 1536 bytes, got %d",len(raw))
	}
	out:=make([]float32,NativeProjectionInputDim)
	for i:=range out{
		out[i]=math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		if math.IsNaN(float64(out[i])) || math.IsInf(float64(out[i]),0){
			return nil,fmt.Errorf("non-finite MiniLM feature %d",i)
		}
	}
	return out,nil
}
