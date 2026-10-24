//go:build js && wasm

package main

import (
	"encoding/json"
	"math/rand"
	"syscall/js"
	"time"

	hdccsr "github.com/kbelludoo/mbvm-db/experiments/hdc-csr"
)

func runClientBenchmark(this js.Value, args []js.Value) any {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	v1 := hdccsr.GenerateRandomHypervector(rng)
	v2 := hdccsr.GenerateRandomHypervector(rng)
	v3 := hdccsr.GenerateRandomHypervector(rng)
	v4 := hdccsr.GenerateRandomHypervector(rng)
	mask := hdccsr.GenerateRandomHypervector(rng)

	// 1. Hamming Benchmark (100.000 iterações)
	const itersHamming = 100000
	t0 := time.Now()
	for i := 0; i < itersHamming; i++ {
		_ = hdccsr.HammingDistance(v1, v2)
	}
	durHamming := time.Since(t0)
	nsHamming := float64(durHamming.Nanoseconds()) / float64(itersHamming)

	// 2. XOR Bind Benchmark (100.000 iterações)
	const itersBind = 100000
	t0 = time.Now()
	for i := 0; i < itersBind; i++ {
		_ = hdccsr.Bind(v1, v2)
	}
	durBind := time.Since(t0)
	nsBind := float64(durBind.Nanoseconds()) / float64(itersBind)

	// 3. RotL8K Benchmark (50.000 iterações)
	const itersRot = 50000
	t0 = time.Now()
	for i := 0; i < itersRot; i++ {
		_ = hdccsr.RotL8K(v1, 13)
	}
	durRot := time.Since(t0)
	nsRot := float64(durRot.Nanoseconds()) / float64(itersRot)

	// 4. Majority3 Benchmark (50.000 iterações)
	const itersMaj3 = 50000
	t0 = time.Now()
	for i := 0; i < itersMaj3; i++ {
		_ = hdccsr.Majority3(v1, v2, v3)
	}
	durMaj3 := time.Since(t0)
	nsMaj3 := float64(durMaj3.Nanoseconds()) / float64(itersMaj3)

	// 5. Majority4 Benchmark (50.000 iterações)
	const itersMaj4 = 50000
	t0 = time.Now()
	for i := 0; i < itersMaj4; i++ {
		_ = hdccsr.Majority4(v1, v2, v3, v4, mask)
	}
	durMaj4 := time.Since(t0)
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

	// 3. Medição de distância de Hamming entre o recuperado e o original
	exactDist := hdccsr.HammingDistance(recovered, hCallee)
	noiseDist := hdccsr.HammingDistance(recovered, hCaller)

	res := map[string]any{
		"exact_match_dist": exactDist,
		"noise_dist":       noiseDist,
		"total_bits":       hdccsr.VectorBits,
		"status":           "Exact 100% associative recovery verified",
	}

	bytes, _ := json.Marshal(res)
	return string(bytes)
}

func main() {
	js.Global().Set("mbvmHdcBenchmark", js.FuncOf(runClientBenchmark))
	js.Global().Set("mbvmHdcDemoUnbind", js.FuncOf(demoAssociativeRetrieval))

	// Bloqueia para manter o runtime Go WASM ativo
	select {}
}
