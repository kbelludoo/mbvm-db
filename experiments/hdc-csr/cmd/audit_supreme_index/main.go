package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/bits"
	"os"
	"sort"
	"strings"
)

const (
	magic = "MBVM8K1\x00"
	bitsPerVector = 8192
	bytesPerVector = 1024
	wordsPerVector = 128
)

type IndexAudit struct {
	Schema string `json:"schema"`
	File string `json:"file"`
	FileSHA256 string `json:"file_sha256"`
	ExpectedSHA256Matched *bool `json:"expected_sha256_matched,omitempty"`
	Version uint32 `json:"version"`
	HeaderBytes int `json:"header_bytes"`
	VectorBits uint16 `json:"vector_bits"`
	VectorCount uint32 `json:"vector_count"`
	FileBytes int `json:"file_bytes"`
	ExpectedFileBytes int64 `json:"expected_file_bytes"`
	ExactDuplicateVectors int `json:"exact_duplicate_vectors"`
	ConstantBitPositions int `json:"constant_bit_positions"`
	MeanBitOneFraction float64 `json:"mean_bit_one_fraction"`
	SampledQueries int `json:"sampled_queries"`
	MeanNearestNonSelfHamming float64 `json:"mean_nearest_nonself_hamming"`
	MinNearestNonSelfHamming int `json:"min_nearest_nonself_hamming"`
	MaxNearestNonSelfHamming int `json:"max_nearest_nonself_hamming"`
	MeanRandomPairHamming float64 `json:"mean_random_pair_hamming"`
	BitOrder string `json:"bit_order"`
	ProjectionWeightsEmbedded bool `json:"projection_weights_embedded"`
	Limitations []string `json:"limitations"`
}

func hamming(a,b []byte) int {
	total:=0
	for i:=0;i<bytesPerVector;i+=8 {
		total+=bits.OnesCount64(binary.LittleEndian.Uint64(a[i:i+8])^binary.LittleEndian.Uint64(b[i:i+8]))
	}
	return total
}

func inspect(data []byte, maxVectors, querySamples int)(IndexAudit,error) {
	var report IndexAudit
	report.Schema="mbvm-index-audit-v1"
	if len(data)<18 {
		return report,fmt.Errorf("file is shorter than minimum 18-byte header")
	}
	if string(data[:8])!=magic {
		return report,fmt.Errorf("incorrect MBVM8K1 binary magic")
	}
	report.Version=binary.LittleEndian.Uint32(data[8:12])
	report.VectorCount=binary.LittleEndian.Uint32(data[12:16])
	report.VectorBits=binary.LittleEndian.Uint16(data[16:18])
	switch report.Version{
	case 1:report.HeaderBytes=18
	case 2:report.HeaderBytes=64
	default:return report,fmt.Errorf("unsupported index version %d (only v1 and v2)",report.Version)
	}
	if report.VectorBits!=bitsPerVector {
		return report,fmt.Errorf("unexpected %d-bit dimensions",report.VectorBits)
	}
	if report.VectorCount==0 || uint64(report.VectorCount)>uint64(maxVectors) {
		return report,fmt.Errorf("vector count %d invalid or exceeds -max-vectors %d",report.VectorCount,maxVectors)
	}
	report.FileBytes=len(data)
	report.ExpectedFileBytes=int64(report.HeaderBytes)+int64(report.VectorCount)*bytesPerVector
	if int64(len(data))!=report.ExpectedFileBytes {
		return report,fmt.Errorf("header declares %d vectors, but file size %d differs from required %d",
			report.VectorCount,len(data),report.ExpectedFileBytes)
	}
	if report.Version==2 {
		for i:=18;i<64;i++ {
			if data[i]!=0 {return report,fmt.Errorf("nonzero v2 header padding byte %d",i)}
		}
	}
	hash:=sha256.Sum256(data)
	report.FileSHA256=hex.EncodeToString(hash[:])
	report.BitOrder="unknown: binary header has no bit-order field; producer metadata or a known test vector is required"
	report.ProjectionWeightsEmbedded=false
	report.Limitations=[]string{
		"The index stores one 1024-byte binary vector per node; trained 384x8192 float32 weights are NOT included.",
		"The file cannot establish its training corpus, provenance, learned semantics, or metadata for node IDs.",
		"Hamming distances and duplicate counts are geometric diagnostics, not evidence of code equivalence.",
		"The v1/v2 header does not specify whether numpy.packbits used big or little bit ordering.",
		"Go heap allocation, resident memory, and disk file size are different quantities.",
	}
	N:=int(report.VectorCount)
	vec:=func(i int) []byte {
		start:=report.HeaderBytes+i*bytesPerVector
		return data[start:start+bytesPerVector]
	}
	dup:=make(map[[32]byte]bool,N)
	var ones [bitsPerVector]uint32
	for i:=0;i<N;i++ {
		v:=vec(i)
		h:=sha256.Sum256(v)
		if dup[h] {report.ExactDuplicateVectors++}
		dup[h]=true
		for offset,b:=range v {
			for bit:=0;bit<8;bit++ {
				if b&(1<<uint(bit))!=0{ones[offset*8+bit]++}
			}
		}
	}
	totalOnes:=uint64(0)
	for _,count:=range ones {
		totalOnes+=uint64(count)
		if count==0 || count==report.VectorCount{report.ConstantBitPositions++}
	}
	report.MeanBitOneFraction=float64(totalOnes)/float64(N*bitsPerVector)
	if N<2{return report,nil}
	if querySamples<1 {return report,fmt.Errorf("queries must be >=1")}
	if querySamples>N {querySamples=N}
	report.SampledQueries=querySamples
	nearest:=make([]int,querySamples)
	for q:=0;q<querySamples;q++ {
		id:=(q*N)/querySamples // Deterministic and never duplicates while samples <= N.
		minDist:=bitsPerVector+1
		for candidate:=0;candidate<N;candidate++ {
			if candidate==id{continue}
			d:=hamming(vec(id),vec(candidate))
			if d<minDist {minDist=d}
		}
		nearest[q]=minDist
	}
	sort.Ints(nearest)
	var sum float64
	for _,distance:=range nearest{sum+=float64(distance)}
	report.MinNearestNonSelfHamming=nearest[0]
	report.MaxNearestNonSelfHamming=nearest[len(nearest)-1]
	report.MeanNearestNonSelfHamming=sum/float64(len(nearest))
	// Same deterministic size as the number of queries, but different
	// query/reference pairs to expose typical distances, not just neighbors.
	randomSum:=0
	for i:=0;i<querySamples;i++ {
		left:=(i*N)/querySamples
		right:=(left+N/2+1)%N
		if right==left{right=(right+1)%N}
		randomSum+=hamming(vec(left),vec(right))
	}
	report.MeanRandomPairHamming=float64(randomSum)/float64(querySamples)
	return report,nil
}

func main(){
	file:=flag.String("file","output/knowledge_8k_supreme.bin","binary index downloaded from Kaggle")
	expected:=flag.String("expected-sha256","","expected SHA-256 from an independent trusted source")
	maxNodes:=flag.Int("max-vectors",100000,"safety cap on vector count")
	samples:=flag.Int("queries",32,"count of deterministic sampled nearest-neighbor searches")
	out:=flag.String("out","","optional JSON audit destination")
	flag.Parse()
	if *maxNodes<=0 || *samples<=0 {
		fmt.Fprintln(os.Stderr,"-max-vectors and -queries must be positive")
		os.Exit(2)
	}
	info,err:=os.Stat(*file)
	if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
	if info.Size()<18 || info.Size()>int64(*maxNodes)*bytesPerVector+64 {
		fmt.Fprintf(os.Stderr,"size %d exceeds declared safety bounds\n",info.Size())
		os.Exit(1)
	}
	data,err:=os.ReadFile(*file)
	if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
	report,err:=inspect(data,*maxNodes,*samples)
	if err!=nil{fmt.Fprintln(os.Stderr,"index audit failed:",err);os.Exit(1)}
	report.File=*file
	if *expected!=""{
		match:=strings.EqualFold(*expected,report.FileSHA256)
		report.ExpectedSHA256Matched=&match
		if !match {
			fmt.Fprintf(os.Stderr,"SHA-256 does not match expected value: got %s\n",report.FileSHA256)
			os.Exit(1)
		}
	}
	result,err:=json.MarshalIndent(report,"","  ")
	if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
	result=append(result,'\n')
	if *out!=""{
		if err=os.WriteFile(*out,result,0644);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
	}
	os.Stdout.Write(result)
}
