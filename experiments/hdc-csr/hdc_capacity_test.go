package hdccsr

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
)

// This capacity stress test reuses the exact production HDC Bind, Majority,
// Unbind and HammingDistance operations. It deliberately extends the sequence
// length beyond the previous 3..5-step test. "Fixed bytes" describes the
// physical representation, NOT lossless capacity for arbitrary programs.

const capacityMaxLength = 64

type capacityCodebook struct {
	roles [capacityMaxLength]Hypervector
	operations [compOps]Hypervector
	tie Hypervector
}

func newCapacityCodebook(seed int64) *capacityCodebook {
	rng := rand.New(rand.NewSource(seed))
	book := &capacityCodebook{tie: GenerateRandomHypervector(rng)}
	for i := range book.roles {
		book.roles[i] = GenerateRandomHypervector(rng)
	}
	for i := range book.operations {
		book.operations[i] = GenerateRandomHypervector(rng)
	}
	return book
}

func (book *capacityCodebook) closest(v Hypervector) uint8 {
	best, dist := uint8(0), VectorBits+1
	for id, candidate := range book.operations {
		d := HammingDistance(v,candidate)
		if d < dist {
			best,dist = uint8(id),d
		}
	}
	return best
}

func (book *capacityCodebook) encode(program dynamicProgram) Hypervector {
	messages := make([]Hypervector, len(program))
	for i, op := range program {
		messages[i] = Bind(book.roles[i],book.operations[op])
	}
	bundle,ok := Majority(messages,book.tie)
	if !ok {panic("cannot bundle empty program")}
	return bundle
}

func (book *capacityCodebook) decode(v Hypervector, length int) dynamicProgram {
	result:=make(dynamicProgram,length)
	for i:=range result {
		result[i] = book.closest(Unbind(v,book.roles[i]))
	}
	return result
}

func (book *capacityCodebook) separate(program dynamicProgram, noise float64, seed int64) dynamicProgram {
	result:=make(dynamicProgram,len(program))
	for i,op:=range program {
		// Same vector dimension, same bit-flip fraction; total storage is
		// LENGTH*1024 bytes, not fixed at 1024 bytes.
		v:=compCorrupt(book.operations[op],noise,seed+int64(i)*131071)
		result[i]=book.closest(v)
	}
	return result
}

func capacityPrograms(length, count int, seed int64) []dynamicProgram {
	rng:=rand.New(rand.NewSource(seed))
	programs:=make([]dynamicProgram,0,count)
	seen:=make(map[string]bool,count)
	for len(programs)<count {
		p:=make(dynamicProgram,length)
		for i:=range p {p[i]=uint8(rng.Intn(compOps))}
		key:=string(p)
		if seen[key]{continue}
		seen[key]=true
		programs=append(programs,p)
	}
	return programs
}

type capacityMetric struct {
	CorrectSteps int `json:"correct_steps"`
	TotalSteps int `json:"total_steps"`
	StepAccuracy float64 `json:"step_accuracy"`
	ExactPrograms int `json:"exact_programs"`
	TotalPrograms int `json:"total_programs"`
	ExactProgramRate float64 `json:"exact_program_rate"`
	CorrectAnswers int `json:"correct_answers"`
	TotalAnswers int `json:"total_answers"`
	AnswerAccuracy float64 `json:"answer_accuracy"`
}

type capacityRow struct {
	Seed int64 `json:"seed"`
	Length int `json:"length"`
	Noise float64 `json:"bit_flip_rate"`
	BundleBytes int `json:"superposed_bytes"`
	SymbolicBytes int `json:"symbolic_bytes"`
	SeparateBytes int `json:"separate_codewords_bytes"`
	Superposed capacityMetric `json:"superposed_hdc"`
	Separate capacityMetric `json:"separate_codewords"`
	Repetition capacityMetric `json:"equal_budget_repetition_code"`
}
type capacityReport struct {
	Schema string `json:"schema"`
	ProgramCountPerLength int `json:"program_count_per_length"`
	CompositionsPreStored int `json:"compositions_pre_stored"`
	CompositionTrainingLabels int `json:"composition_training_labels"`
	Interpretation string `json:"interpretation"`
	Results []capacityRow `json:"results"`
}

// Two bits per primitive and deterministic repetition over the SAME
// 8192-bit storage budget. This is a conventional equal-size ECC control.
func capacityRepeatEncode(program dynamicProgram) Hypervector {
	var out Hypervector
	logicalBits:=2*len(program)
	for logical:=0;logical<logicalBits;logical++{
		if program[logical/2]&(1<<uint(logical%2))==0 {continue}
		start:=logical*VectorBits/logicalBits
		end:=(logical+1)*VectorBits/logicalBits
		for bit:=start;bit<end;bit++{
			out[bit/64]|=uint64(1)<<uint(bit%64)
		}
	}
	return out
}
func capacityRepeatDecode(v Hypervector,length int)dynamicProgram {
	p:=make(dynamicProgram,length)
	logicalBits:=2*length
	for logical:=0;logical<logicalBits;logical++{
		start:=logical*VectorBits/logicalBits
		end:=(logical+1)*VectorBits/logicalBits
		ones:=0
		for bit:=start;bit<end;bit++{
			if v[bit/64]&(uint64(1)<<uint(bit%64))!=0 {ones++}
		}
		if ones*2>=(end-start) {p[logical/2]|=1<<uint(logical%2)}
	}
	return p
}

func capacityEvaluate(expected, decoded dynamicProgram, metric *capacityMetric, fitted [compOps]compAffine) {
	metric.TotalPrograms++
	correct:=true
	for i:=range expected {
		metric.TotalSteps++
		if decoded[i]==expected[i]{metric.CorrectSteps++}else{correct=false}
	}
	if correct{metric.ExactPrograms++}
	// Numerical evaluation is a supporting metric: wrong programs sometimes
	// happen to produce the same answer through operation cancellation.
	// Never use it as a replacement for exact ordered-step recovery.
	for _,x:=range []float64{11,17,30}{
		metric.TotalAnswers++
		got:=x
		want:=x
		for _,op:=range decoded{got=fitted[op].apply(got)}
		for _,op:=range expected{want=compTrue(int(op),want)}
		if math.Round(got)==want{metric.CorrectAnswers++}
	}
}

func capacityFinalize(m capacityMetric) capacityMetric {
	m.StepAccuracy=float64(m.CorrectSteps)/float64(m.TotalSteps)
	m.ExactProgramRate=float64(m.ExactPrograms)/float64(m.TotalPrograms)
	m.AnswerAccuracy=float64(m.CorrectAnswers)/float64(m.TotalAnswers)
	return m
}

func TestHDCSuperpositionCapacityStress(t *testing.T){
	const count=12
	lengths:=[]int{3,5,8,10,12,16,24,32,48,64}
	noises:=[]float64{0,0.1,0.2,0.4}
	seeds:=[]int64{2026,2027,2028}
	var fitted [compOps]compAffine
	for i:=range fitted{fitted[i]=compLearnAtomic(i)}
	result:=capacityReport{
		Schema:"mbvm-hdc-capacity-v1",
		ProgramCountPerLength:count,
		CompositionsPreStored:0,
		CompositionTrainingLabels:0,
		Interpretation:"Stress test: fixed 1024-byte bundle does not imply arbitrarily recoverable depth. Query supplies operation IDs and executor is given. Compare to equal-budget 1024-byte repetition ECC, unbundled codeword per step and uncorrupted byte list. No semantic planning is evaluated.",
		Results:make([]capacityRow,0,len(lengths)*len(noises)*len(seeds)),
	}
	for _,seed:=range seeds {
		book:=newCapacityCodebook(seed)
		for _,length:=range lengths {
			programs:=capacityPrograms(length,count,2026000+int64(length))
			for _,noise:=range noises{
				var hdc,separate,repetition capacityMetric
				for i,p:=range programs {
					// Encode once per query (never store the full sequence);
					// each noise setting perturbs the same exact clean code.
					bundle:=book.encode(p)
					noiseSeed:=seed*1000000+int64(length*10000+i*97)
					corrupted:=compCorrupt(bundle,noise,noiseSeed)
					fromHDC:=book.decode(corrupted,length)
					fromSeparate:=book.separate(p,noise,noiseSeed)
					repeat:=capacityRepeatEncode(p)
					fromRepetition:=capacityRepeatDecode(compCorrupt(repeat,noise,noiseSeed),length)
					capacityEvaluate(p,fromHDC,&hdc,fitted)
					capacityEvaluate(p,fromSeparate,&separate,fitted)
					capacityEvaluate(p,fromRepetition,&repetition,fitted)
				}
				row:=capacityRow{
					Seed:seed,Length:length,Noise:noise,
					BundleBytes:WordsPerVector*8,SymbolicBytes:length,
					SeparateBytes:length*WordsPerVector*8,
					Superposed:capacityFinalize(hdc),
					Separate:capacityFinalize(separate),
					Repetition:capacityFinalize(repetition),
				}
				result.Results=append(result.Results,row)
				t.Logf("seed=%d steps=%d flip=%2.0f%%: HDC step=%.1f%% complete=%.1f%% numeric=%.1f%%; separate complete=%.1f%%; ECC1024 complete=%.1f%%",
					seed,length,noise*100,
					100*row.Superposed.StepAccuracy,100*row.Superposed.ExactProgramRate,
					100*row.Superposed.AnswerAccuracy,100*row.Separate.ExactProgramRate,
					100*row.Repetition.ExactProgramRate)
				if noise==0 && length==3 && row.Superposed.ExactPrograms==0 {
					t.Fatal("clean 3-step control failed completely")
				}
				if row.BundleBytes!=1024 || row.SymbolicBytes!=length ||
					row.SeparateBytes!=length*1024 {
					t.Fatal("incorrect representation size accounting")
				}
			}
		}
	}
	if len(result.Results)!=120 {
		t.Fatalf("expected 120 settings, got %d",len(result.Results))
	}
	raw,err:=json.MarshalIndent(result,"","  ")
	if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile("hdc_capacity_results.json",raw,0644);err!=nil{t.Fatal(err)}
}
