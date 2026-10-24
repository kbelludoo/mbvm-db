package hdccsr

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
)

// Dynamic composition v2: no length-3..5 program is stored as a memory item.
// Each query provides an ordered sequence of primitive IDs. The four affine
// primitives were fitted ONLY on atomic data in compLearnAtomic.
//
// The HDC branch encodes this GIVEN sequence into one 8192-bit majority
// bundle of position-bound atomic codes; it then decodes the list of IDs and
// runs the identical learned affine executor used by both controls.
//
// Important: encoding a given program != inventing the program from intent.

type dynamicProgram []uint8

func dynamicPrograms(length, count int, seed int64) []dynamicProgram {
	capacity := 1
	for i := 0; i < length; i++ {
		capacity *= compOps
	}
	if count > capacity {
		panic("requested too many unique programs")
	}
	all := make([]int, capacity)
	for i := range all { all[i] = i }
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(all), func(i,j int) { all[i],all[j] = all[j],all[i] })
	result := make([]dynamicProgram, 0, count)
	for _,code := range all[:count] {
		seq := make(dynamicProgram, length)
		for i := length-1; i >= 0; i-- {
			seq[i] = uint8(code%compOps)
			code /= compOps
		}
		result = append(result, seq)
	}
	return result
}

type dynamicCodebook struct {
	roles [5]Hypervector
	ops [compOps]Hypervector
	tie Hypervector
}

func newDynamicCodebook(seed int64) *dynamicCodebook {
	rng := rand.New(rand.NewSource(seed))
	book := &dynamicCodebook{tie: GenerateRandomHypervector(rng)}
	for i := range book.roles { book.roles[i] = GenerateRandomHypervector(rng) }
	for i := range book.ops { book.ops[i] = GenerateRandomHypervector(rng) }
	return book
}

func (book *dynamicCodebook) nearest(candidate Hypervector) uint8 {
	best, bestD := uint8(0), VectorBits+1
	for op,v := range book.ops {
		if d := HammingDistance(candidate, v); d < bestD {
			bestD, best = d, uint8(op)
		}
	}
	return best
}

// The bundle is created FROM THE CURRENT QUERY, not fetched from a table
// preloaded with compositions. The same clean bundle is reused at each noise
// rate, allowing paired noise comparisons.
func (book *dynamicCodebook) encode(program dynamicProgram) Hypervector {
	if len(program) < 3 || len(program) > len(book.roles) {
		panic("dynamic program length must be between 3 and 5")
	}
	msgs := make([]Hypervector,len(program))
	for i,op := range program {
		if op >= compOps { panic("invalid operation") }
		msgs[i] = Bind(book.roles[i], book.ops[op])
	}
	bundle, ok := Majority(msgs,book.tie)
	if !ok { panic("empty HDC bundle") }
	return bundle
}

func (book *dynamicCodebook) decode(bundle Hypervector, length int) dynamicProgram {
	p := make(dynamicProgram,length)
	for i := range p {
		p[i] = book.nearest(Unbind(bundle,book.roles[i]))
	}
	return p
}

func (book *dynamicCodebook) decodeSeparate(program dynamicProgram,rate float64,seed int64) dynamicProgram {
	// Strong non-superposed HDC codebook control: use the SAME item vectors,
	// but store one independent 8192-bit vector per step (3-5 KB/query).
	// Bit-flips are independent per vector, at the same corruption fraction.
	decoded := make(dynamicProgram,len(program))
	for i,op := range program {
		vec := compCorrupt(book.ops[op],rate,seed+int64(i)*131071)
		decoded[i] = book.nearest(vec)
	}
	return decoded
}

func dynamicLearned(models [compOps]compAffine, program dynamicProgram, x float64) float64 {
	for _,op := range program { x = models[op].apply(x) }
	return x
}

func dynamicOracle(program dynamicProgram, x float64) float64 {
	for _,op := range program { x = compTrue(int(op),x) }
	return x
}

type dynamicMetrics struct {
	Correct int `json:"correct"`
	Total int `json:"total"`
	ExactAnswerRate float64 `json:"exact_answer_rate"`
	MeanAbsoluteError float64 `json:"mean_absolute_error"`
	StepMatches int `json:"step_matches"`
	TotalSteps int `json:"total_steps"`
	StepAccuracy float64 `json:"step_accuracy"`
	EntireProgramsRecovered int `json:"entire_programs_recovered"`
	TotalPrograms int `json:"total_programs"`
}

type dynamicAccumulator struct{
	dynamicMetrics
}

func (m *dynamicAccumulator) add(actual,decoded dynamicProgram, models [compOps]compAffine, inputs []float64){
	m.TotalPrograms++
	exactProgram:=true
	for i:=range actual {
		m.TotalSteps++
		if actual[i]==decoded[i] {
			m.StepMatches++
		} else { exactProgram=false }
	}
	if exactProgram {m.EntireProgramsRecovered++}
	for _,x:=range inputs {
		y:=dynamicOracle(actual,x)
		got:=dynamicLearned(models,decoded,x)
		m.Total++
		if math.Round(got)==y {m.Correct++}
		m.MeanAbsoluteError+=math.Abs(got-y)
	}
}

func(m *dynamicAccumulator) result()dynamicMetrics{
	d:=m.dynamicMetrics
	d.ExactAnswerRate=float64(d.Correct)/float64(d.Total)
	d.MeanAbsoluteError/=float64(d.Total)
	d.StepAccuracy=float64(d.StepMatches)/float64(d.TotalSteps)
	return d
}

type dynamicTrial struct {
	Seed int64 `json:"seed"`
	Length int `json:"program_length"`
	Noise float64 `json:"bit_flip_rate"`
	ProgramCount int `json:"program_count"`
	Queries int `json:"numeric_queries"`
	ExplicitList dynamicMetrics `json:"learned_atomic_with_given_sequence"`
	HDCSuperposition dynamicMetrics `json:"learned_atomic_hdc_superposed_sequence"`
	SeparateCodewords dynamicMetrics `json:"learned_atomic_separate_codewords"`
	ExplicitListBytesPerProgram int `json:"explicit_list_bytes_per_program"`
	HDCBundleBytesPerProgram int `json:"hdc_bundle_bytes_per_program"`
	SeparateCodewordBytesPerProgram int `json:"separate_codewords_bytes_per_program"`
}

type dynamicReport struct {
	Schema string `json:"schema"`
	AtomicTrainingRange string `json:"atomic_training_range"`
	ComposedTrainingLabels int `json:"composed_training_labels"`
	ProgramsPreStored int `json:"programs_pre_stored"`
	TotalTrials int `json:"total_trials"`
	Results []dynamicTrial `json:"results"`
	Interpretation string `json:"interpretation"`
}

func TestAlgebraLabDynamicUnstoredComposition(t *testing.T){
	var models [compOps]compAffine
	for i:=range models { models[i]=compLearnAtomic(i) }
	// Numeric OOD input sequence disjoint from atomic [-10,10] fit examples.
	inputs:=[]float64{11,14,17,23,30}
	report:=dynamicReport{
		Schema:"mbvm-dynamic-composition-v1",
		AtomicTrainingRange:"integer [-10,10] only",
		ComposedTrainingLabels:0,
		ProgramsPreStored:0,
		Results:make([]dynamicTrial,0,3*3*4),
		Interpretation:"No composed programs are stored; task provides step IDs and an explicit executor applies learned affine rules. HDC only superposes and recovers the provided step list. Compare to exact list and independent codeword baselines; this does not show rule discovery or unique HDC advantage.",
	}
	for _,seed:=range []int64{2026,2027,2028} {
		book:=newDynamicCodebook(seed)
		for length:=3;length<=5;length++{
			// Same program sequence across codebook seeds and noise levels.
			programs:=dynamicPrograms(length,32,777+int64(length))
			for _,rate:=range []float64{0,0.1,0.2,0.4} {
				var direct,superposed,separate dynamicAccumulator
				for task,program:=range programs{
					clean:=book.encode(program)
					noiseSeed:=seed*100000+int64(length*1000+task)
					corrupted:=compCorrupt(clean,rate,noiseSeed)
					recHDC:=book.decode(corrupted,length)
					recSeparate:=book.decodeSeparate(program,rate,noiseSeed)
					direct.add(program,program,models,inputs)
					superposed.add(program,recHDC,models,inputs)
					separate.add(program,recSeparate,models,inputs)
				}
				row:=dynamicTrial{
					Seed:seed, Length:length, Noise:rate,
					ProgramCount:len(programs),
					Queries:len(programs)*len(inputs),
					ExplicitList:direct.result(),
					HDCSuperposition:superposed.result(),
					SeparateCodewords:separate.result(),
					ExplicitListBytesPerProgram:length,
					HDCBundleBytesPerProgram:WordsPerVector*8,
					SeparateCodewordBytesPerProgram:length*WordsPerVector*8,
				}
				report.Results=append(report.Results,row)
				t.Logf("seed=%d length=%d flip=%.0f%% explicit=%.1f%% HDC=%.1f%% (programs %d/%d) separate=%.1f%% (programs %d/%d)",
					seed,length,100*rate,
					100*row.ExplicitList.ExactAnswerRate,
					100*row.HDCSuperposition.ExactAnswerRate,
					row.HDCSuperposition.EntireProgramsRecovered,len(programs),
					100*row.SeparateCodewords.ExactAnswerRate,
					row.SeparateCodewords.EntireProgramsRecovered,len(programs))
				if direct.result().ExactAnswerRate < 1 {
					t.Fatalf("learned atomic explicit execution failed: length %d seed %d",length,seed)
				}
				if rate==0 && (row.HDCSuperposition.EntireProgramsRecovered!=len(programs)||row.SeparateCodewords.EntireProgramsRecovered!=len(programs)){
					t.Fatalf("clean codebook failed: length %d seed %d, HDC %d/%d separate %d/%d",
						length,seed,row.HDCSuperposition.EntireProgramsRecovered,len(programs),
						row.SeparateCodewords.EntireProgramsRecovered,len(programs))
				}
			}
		}
	}
	report.TotalTrials=len(report.Results)
	if report.TotalTrials!=36 {t.Fatal(fmt.Sprintf("want 36 cases, got %d",report.TotalTrials))}
	blob,err:=json.MarshalIndent(report,"","  ")
	if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile("dynamic_composition_results.json",blob,0644);err!=nil{t.Fatal(err)}
}
