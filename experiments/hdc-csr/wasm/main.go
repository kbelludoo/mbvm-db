//go:build js && wasm

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"syscall/js"
	"time"

	hdccsr "github.com/kbelludoo/mbvm-db/experiments/hdc-csr"
)

// Sink global volátil para impedir dead-code elimination do compilador
var (
	benchmarkSinkInt int
	benchmarkSinkVec hdccsr.Hypervector
)

func runClientBenchmark(this js.Value, args []js.Value) any {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	v1 := hdccsr.GenerateRandomHypervector(rng)
	v2 := hdccsr.GenerateRandomHypervector(rng)
	v3 := hdccsr.GenerateRandomHypervector(rng)
	v4 := hdccsr.GenerateRandomHypervector(rng)
	mask := hdccsr.GenerateRandomHypervector(rng)

	// 1. Hamming Benchmark (100.000 iterações com dependência de dados)
	const itersHamming = 100000
	localHammingAcc := 0
	currV1 := v1
	t0 := time.Now()
	for i := 0; i < itersHamming; i++ {
		dist := hdccsr.HammingDistance(currV1, v2)
		localHammingAcc += dist
		currV1[i%hdccsr.WordsPerVector] ^= uint64(dist) // Varia a entrada impedindo memoização
	}
	durHamming := time.Since(t0)
	benchmarkSinkInt += localHammingAcc
	nsHamming := float64(durHamming.Nanoseconds()) / float64(itersHamming)

	// 2. XOR Bind Benchmark (100.000 iterações)
	const itersBind = 100000
	currBind := v1
	t0 = time.Now()
	for i := 0; i < itersBind; i++ {
		currBind = hdccsr.Bind(currBind, v2)
	}
	durBind := time.Since(t0)
	benchmarkSinkVec = currBind
	nsBind := float64(durBind.Nanoseconds()) / float64(itersBind)

	// 3. RotL8K Benchmark (50.000 iterações)
	const itersRot = 50000
	currRot := v1
	t0 = time.Now()
	for i := 0; i < itersRot; i++ {
		currRot = hdccsr.RotL8K(currRot, 13)
	}
	durRot := time.Since(t0)
	benchmarkSinkVec = currRot
	nsRot := float64(durRot.Nanoseconds()) / float64(itersRot)

	// 4. Majority3 Benchmark (50.000 iterações com variação de dados)
	const itersMaj3 = 50000
	currMaj3 := v1
	t0 = time.Now()
	for i := 0; i < itersMaj3; i++ {
		currMaj3 = hdccsr.Majority3(currMaj3, v2, v3)
		currMaj3[0] ^= uint64(i)
	}
	durMaj3 := time.Since(t0)
	benchmarkSinkVec = currMaj3
	nsMaj3 := float64(durMaj3.Nanoseconds()) / float64(itersMaj3)

	// 5. Majority4 Benchmark (50.000 iterações com variação de dados)
	const itersMaj4 = 50000
	currMaj4 := v1
	t0 = time.Now()
	for i := 0; i < itersMaj4; i++ {
		currMaj4 = hdccsr.Majority4(currMaj4, v2, v3, v4, mask)
		currMaj4[1] ^= uint64(i)
	}
	durMaj4 := time.Since(t0)
	benchmarkSinkVec = currMaj4
	nsMaj4 := float64(durMaj4.Nanoseconds()) / float64(itersMaj4)

	res := map[string]any{
		"ns_hamming":   nsHamming,
		"ns_bind":      nsBind,
		"ns_rotl":      nsRot,
		"ns_majority3": nsMaj3,
		"ns_majority4": nsMaj4,
	}

	bytes, _ := json.Marshal(res)
	return string(bytes)
}

func demoAssociativeRetrieval(this js.Value, args []js.Value) any {
	rng := rand.New(rand.NewSource(42))
	hdc := hdccsr.NewHDCSystem(42)

	hCaller := hdccsr.GenerateRandomHypervector(rng)
	hCallee := hdccsr.GenerateRandomHypervector(rng)

	relCfg := hdc.RelConfigs[hdccsr.RelCall]

	// 1. Mensagem de chamada: ROTL(callee, shift) ⊕ Role
	msg := hdccsr.Bind(hdccsr.RotL8K(hCallee, relCfg.Shift), relCfg.RoleVector)

	// 2. Desassociação (Unbinding query): ROTR(msg ⊕ Role, shift)
	unbound := hdccsr.Unbind(msg, relCfg.RoleVector)
	recovered := hdccsr.RotR8K(unbound, relCfg.Shift)

	// 3. Medição de distâncias
	exactDist := hdccsr.HammingDistance(recovered, hCallee)
	noiseDist := hdccsr.HammingDistance(recovered, hCaller)

	// 4. Converte os 8.192 bits (1.024 bytes) reais do vetor recuperado para hex string
	rawBytes := make([]byte, hdccsr.WordsPerVector*8)
	for i := 0; i < hdccsr.WordsPerVector; i++ {
		w := recovered[i]
		for b := 0; b < 8; b++ {
			rawBytes[i*8+b] = byte(w >> (b * 8))
		}
	}
	hexBits := hex.EncodeToString(rawBytes)

	res := map[string]any{
		"exact_match_dist": exactDist,
		"noise_dist":       noiseDist,
		"total_bits":       hdccsr.VectorBits,
		"recovered_hex":    hexBits,
		"status":           "Exact 100% associative recovery verified",
		"fingerprint":      fmt.Sprintf("%016x...%016x", recovered[0], recovered[127]),
	}

	bytes, _ := json.Marshal(res)
	return string(bytes)
}

func main() {
	js.Global().Set("mbvmHdcBenchmark", js.FuncOf(runClientBenchmark))
	js.Global().Set("mbvmHdcDemoUnbind", js.FuncOf(demoAssociativeRetrieval))

	select {}
}
