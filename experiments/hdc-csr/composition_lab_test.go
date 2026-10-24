package hdccsr

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
)

// Composition Lab v1: The SAME atomic labels are available to all models.
// A: small unconstrained MLP trained on atomic examples only.
// B: learned affine primitives executed with explicit symbolic sequencing.
// C: the SAME learned primitives, but program steps recalled from HDC roles.
// D: the SAME primitives with nearest-key lookup + ordinary map (non-HDC control).
// No composed output label is used anywhere during training.

const compOps = 4
const compIdentity = 4
const compInputs = 11 // [x/10, slot1 one-hot(5), slot2 one-hot(5)]
const compHidden = 32

type compExample struct {
	x, first, second int
	y float64
}

func compTrue(op int, x float64) float64 {
	switch op {
	case 0: return 2*x
	case 1: return x+3
	case 2: return -x
	case 3: return x-4
	case compIdentity: return x
	default: panic("unknown operation")
	}
}

type compAffine struct { w, b float64 }

func compLearnAtomic(op int) compAffine {
	// The architecture is affine, but coefficients are learned solely from
	// integer input-output pairs for this operation, not supplied as rules.
	m:=compAffine{w:0.17,b:-0.21}
	const step=0.004
	for epoch:=0;epoch<4000;epoch++{
		dw,db:=0.0,0.0
		for x:=-10;x<=10;x++{
			err:=m.w*float64(x)+m.b-compTrue(op,float64(x))
			dw+=2*err*float64(x)
			db+=2*err
		}
		m.w-=step*dw/21
		m.b-=step*db/21
	}
	return m
}
func (m compAffine) apply(x float64)float64{return m.w*x+m.b}

type compMLP struct {
	w [compHidden][compInputs]float64
	b [compHidden]float64
	out [compHidden]float64
	bias float64
}

func compFeatures(x,op1,op2 int)[compInputs]float64 {
	var f [compInputs]float64
	f[0]=float64(x)/10.0
	f[1+op1]=1
	f[6+op2]=1
	return f
}
func compNewMLP(seed int64)*compMLP{
	rng:=rand.New(rand.NewSource(seed))
	net:=&compMLP{}
	for h:=0;h<compHidden;h++{
		for i:=0;i<compInputs;i++{net.w[h][i]=(rng.Float64()*2-1)*0.12}
		net.out[h]=(rng.Float64()*2-1)*0.12
	}
	return net
}
func(net *compMLP) forward(f [compInputs]float64)(float64,[compHidden]float64){
	var activations [compHidden]float64
	output:=net.bias
	for h:=0;h<compHidden;h++{
		z:=net.b[h]
		for j:=0;j<compInputs;j++{z+=net.w[h][j]*f[j]}
		activations[h]=math.Tanh(z)
		output+=net.out[h]*activations[h]
	}
	return output,activations
}
func(net *compMLP) fitAtomic(seed int64){
	rng:=rand.New(rand.NewSource(seed))
	train:=make([]compExample,0,compOps*21*2)
	for op:=0;op<compOps;op++{
		for x:=-10;x<=10;x++{
			y:=compTrue(op,float64(x))/20.0
			train=append(train,compExample{x,op,compIdentity,y})
			train=append(train,compExample{x,compIdentity,op,y})
		}
	}
	const learningRate=0.018
	for epoch:=0;epoch<550;epoch++{
		rng.Shuffle(len(train),func(i,j int){train[i],train[j]=train[j],train[i]})
		for _,e:=range train{
			f:=compFeatures(e.x,e.first,e.second)
			y,act:=net.forward(f)
			d:=2.0*(y-e.y)
			for h:=0;h<compHidden;h++{
				prev:=net.out[h]
				net.out[h]-=learningRate*d*act[h]
				grad:=d*prev*(1-act[h]*act[h])
				net.b[h]-=learningRate*grad
				for j:=0;j<compInputs;j++{net.w[h][j]-=learningRate*grad*f[j]}
			}
			net.bias-=learningRate*d
		}
	}
}
func(net *compMLP) predict(x,op1,op2 int)float64{
	y,_:=net.forward(compFeatures(x,op1,op2))
	return y*20.0
}

type compProgram struct{first,second int}

type compHDCMemory struct{
	roleFirst,roleSecond,tie Hypervector
	operationCodes [compOps]Hypervector
	keys []Hypervector
	records []Hypervector
}

func compNewHDC(programs []compProgram,seed int64)*compHDCMemory{
	rng:=rand.New(rand.NewSource(seed))
	h:=&compHDCMemory{
		roleFirst:GenerateRandomHypervector(rng),
		roleSecond:GenerateRandomHypervector(rng),
		tie:GenerateRandomHypervector(rng),
		keys:make([]Hypervector,len(programs)),
		records:make([]Hypervector,len(programs)),
	}
	for i:=range h.operationCodes{h.operationCodes[i]=GenerateRandomHypervector(rng)}
	for i,p:=range programs{
		h.keys[i]=GenerateRandomHypervector(rng)
		bundle,_:=Majority([]Hypervector{
			Bind(h.roleFirst,h.operationCodes[p.first]),
			Bind(h.roleSecond,h.operationCodes[p.second]),
		},h.tie)
		h.records[i]=Bind(h.keys[i],bundle)
	}
	return h
}
func(h *compHDCMemory) nearestOp(query Hypervector)int{
	best,bestD:=-1,VectorBits+1
	for op,code:=range h.operationCodes{
		d:=HammingDistance(query,code)
		if d<bestD {best,bestD=op,d}
	}
	return best
}
func(h *compHDCMemory) decode(task int,queryKey Hypervector)compProgram{
	// Source record identified by the task ID; the HDC mechanism is used
	// for PROGRAM reconstruction, not for discovery of a task's ID.
	bundle:=Unbind(h.records[task],queryKey)
	return compProgram{
		first:h.nearestOp(Unbind(bundle,h.roleFirst)),
		second:h.nearestOp(Unbind(bundle,h.roleSecond)),
	}
}
func(h *compHDCMemory) nearestKey(key Hypervector)int{
	best,bestD:=-1,VectorBits+1
	for i,item:=range h.keys{
		d:=HammingDistance(key,item)
		if d<bestD{best,bestD=i,d}
	}
	return best
}

func compCorrupt(key Hypervector,rate float64,seed int64)Hypervector{
	n:=int(rate*VectorBits)
	rng:=rand.New(rand.NewSource(seed))
	indices:=rng.Perm(VectorBits)
	for i:=0;i<n;i++{
		bit:=indices[i]
		key[bit/64]^=uint64(1)<<uint(bit%64)
	}
	return key
}
func compApplyLearned(models [compOps]compAffine,program compProgram,x float64)float64{
	return models[program.second].apply(models[program.first].apply(x))
}
func compExpected(program compProgram,x float64)float64{
	return compTrue(program.second,compTrue(program.first,x))
}

type compMetric struct{
	Correct int `json:"correct"`
	Total int `json:"total"`
	Accuracy float64 `json:"accuracy"`
	MAE float64 `json:"mean_absolute_error"`
}
func compEvaluate(values []float64,expected []float64) compMetric{
	m:=compMetric{Total:len(expected)}
	for i,v:=range values{
		if math.Round(v)==expected[i]{m.Correct++}
		m.MAE+=math.Abs(v-expected[i])/float64(len(expected))
	}
	m.Accuracy=float64(m.Correct)/float64(len(expected))
	return m
}
type compCaseReport struct{
	Seed int64 `json:"seed"`
	NoiseRate float64 `json:"noise_rate"`
	Queries int `json:"queries"`
	UnconstrainedMLP compMetric `json:"neural_mlp_direct"`
	AffineSymbolic compMetric `json:"learned_affine_with_explicit_composition"`
	AffineHDC compMetric `json:"learned_affine_with_hdc_program_memory"`
	AffineNearestKey compMetric `json:"learned_affine_with_nearest_key_map"`
	HDCProgramRecovered int `json:"hdc_program_recovered"`
	MapProgramRecovered int `json:"nearest_key_program_recovered"`
}
type compLabReport struct{
	Schema string `json:"schema"`
	AtomicTrainingRange string `json:"atomic_training_range"`
	OODRange string `json:"ood_range"`
	CompositionTrainingLabels int `json:"composition_training_labels"`
	ProgramCount int `json:"program_count"`
	Interpretation string `json:"interpretation"`
	Results []compCaseReport `json:"results"`
}

// This is a TRAINING/COMPOSITION CONTROL, not proof of general intelligence.
// In particular, the affine symbolic and HDC arms BOTH use an explicit
// executor with the learned primitives. HDC must be compared with the
// nearest-key map to test if its memory representation adds value.
func TestAlgebraLabCompositionalOODTriplet(t *testing.T){
	programs:=make([]compProgram,0,compOps*compOps)
	for a:=0;a<compOps;a++{
		for b:=0;b<compOps;b++{
			programs=append(programs,compProgram{a,b})
		}
	}
	report:=compLabReport{
		Schema:"mbvm-compositional-ood-v1",
		AtomicTrainingRange:"[-10,10], 21 numbers x 4 operations x 2 singleton positions",
		OODRange:"[11,30], 20 numbers x 16 two-operation programs",
		CompositionTrainingLabels:0,
		ProgramCount:len(programs),
		Interpretation:"Toy affine function composition; HDC memorizes task programs, not arithmetic. Compare HDC against nearest-key exact map. No Kaggle neural embeddings or general reasoning claim.",
	}
	var models [compOps]compAffine
	for op:=0;op<compOps;op++{
		models[op]=compLearnAtomic(op)
	}
	for _,seed:=range []int64{2026,2027,2028}{
		mlp:=compNewMLP(seed)
		mlp.fitAtomic(seed)
		hdc:=compNewHDC(programs,seed)
		for _,noise:=range []float64{0,0.2,0.4}{
			var neural,algebra,withHDC,withNearest,expected []float64
			hdcRecovered,mapRecovered:=0,0
			for task,p:=range programs{
				query:=compCorrupt(hdc.keys[task],noise,seed*100000+int64(task)*997)
				fromHDC:=hdc.decode(task,query)
				fromMap:=programs[hdc.nearestKey(query)]
				if fromHDC==p{hdcRecovered++}
				if fromMap==p{mapRecovered++}
				for x:=11;x<=30;x++{
					expected=append(expected,compExpected(p,float64(x)))
					neural=append(neural,mlp.predict(x,p.first,p.second))
					algebra=append(algebra,compApplyLearned(models,p,float64(x)))
					withHDC=append(withHDC,compApplyLearned(models,fromHDC,float64(x)))
					withNearest=append(withNearest,compApplyLearned(models,fromMap,float64(x)))
				}
			}
			row:=compCaseReport{
				Seed:seed,NoiseRate:noise,Queries:len(expected),
				UnconstrainedMLP:compEvaluate(neural,expected),
				AffineSymbolic:compEvaluate(algebra,expected),
				AffineHDC:compEvaluate(withHDC,expected),
				AffineNearestKey:compEvaluate(withNearest,expected),
				HDCProgramRecovered:hdcRecovered,
				MapProgramRecovered:mapRecovered,
			}
			report.Results=append(report.Results,row)
			t.Logf("seed=%d noise=%.0f%%: rawMLP=%.2f%% learnedAffine=%.2f%% HDC-memory=%.2f%% nearestKey-map=%.2f%% (HDC program %d/%d, map %d/%d)",
				seed,noise*100,row.UnconstrainedMLP.Accuracy*100,
				row.AffineSymbolic.Accuracy*100,row.AffineHDC.Accuracy*100,
				row.AffineNearestKey.Accuracy*100,hdcRecovered,len(programs),mapRecovered,len(programs))
			if noise==0 && (row.HDCProgramRecovered!=len(programs)||row.MapProgramRecovered!=len(programs)){
				t.Fatal("clean program retrieval failed; the fixture is invalid")
			}
			if row.AffineSymbolic.Accuracy<0.99{
				t.Fatalf("learned affine reference failed seed=%d accuracy=%.3f",seed,row.AffineSymbolic.Accuracy)
			}
		}
	}
	raw,err:=json.MarshalIndent(report,"","  ")
	if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile("composition_lab_results.json",raw,0644);err!=nil{t.Fatal(err)}
	if len(report.Results)!=9{t.Fatal(fmt.Sprintf("expected nine seed/noise settings; got %d",len(report.Results)))}
}
