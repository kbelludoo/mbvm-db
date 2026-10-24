package hdccsr

import (
	"math/rand"
	"testing"
)

func generateRandomHypervector(rng *rand.Rand) Hypervector {
	var v Hypervector
	for i := 0; i < WordsPerVector; i++ {
		v[i] = rng.Uint64()
	}
	return v
}

func TestBindUnbindInvertibility(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	a := generateRandomHypervector(rng)
	b := generateRandomHypervector(rng)

	bound := Bind(a, b)
	unbound := Unbind(bound, b)

	if unbound != a {
		t.Fatalf("BIND/UNBIND falhou na propriedade de inversão: unbound != a")
	}

	// Quase-ortogonalidade: distância de Hamming esperada em torno de 4.096 bits (± 256 bits)
	dist := HammingDistance(a, bound)
	if dist < 3800 || dist > 4400 {
		t.Errorf("BIND não produziu quase-ortogonalidade esperada em 8.192 bits: dist=%d", dist)
	}
}

func TestRotLRotRInvertibility(t *testing.T) {
	rng := rand.New(rand.NewSource(1337))
	v := generateRandomHypervector(rng)

	testShifts := []int{0, 1, 7, 63, 64, 65, 127, 512, 4096, 8191, 8192, 9000, -1, -65}
	for _, k := range testShifts {
		rot := RotL8K(v, k)
		recovered := RotR8K(rot, k)
		if recovered != v {
			t.Fatalf("ROTL/ROTR falhou para shift %d", k)
		}

		// Rotação não nula e não múltipla de 8.192 deve ser quase-ortogonal ao vetor original
		normalizedK := ((k % VectorBits) + VectorBits) % VectorBits
		if normalizedK > 64 && normalizedK < (VectorBits-64) {
			dist := HammingDistance(v, rot)
			if dist < 3800 || dist > 4400 {
				t.Errorf("ROTL(%d) produziu distância atípica em relação ao original: dist=%d", k, dist)
			}
		}
	}
}

func TestMajorityDegree0(t *testing.T) {
	mask := Hypervector{}
	res, hasContext := Majority(nil, mask)
	if hasContext {
		t.Fatalf("Grau 0 deveria retornar hasContext=false")
	}
	if res != (Hypervector{}) {
		t.Fatalf("Grau 0 deveria retornar vetor zerado")
	}
}

func TestMajorityDegree1(t *testing.T) {
	rng := rand.New(rand.NewSource(100))
	msg := generateRandomHypervector(rng)
	mask := generateRandomHypervector(rng)

	res, hasContext := Majority([]Hypervector{msg}, mask)
	if !hasContext {
		t.Fatalf("Grau 1 deveria retornar hasContext=true")
	}
	if res != msg {
		t.Fatalf("Grau 1 deve retornar exatamente a mensagem do único vizinho sem modificação")
	}
}

func TestMajorityDegree2DeterministicTieBreak(t *testing.T) {
	rng := rand.New(rand.NewSource(200))
	mask := generateRandomHypervector(rng)

	var msg1, msg2 Hypervector
	// Criamos propositalmente bits opostos em msg1 e msg2
	for i := 0; i < WordsPerVector; i++ {
		msg1[i] = 0xAAAAAAAAAAAAAAAA // 10101010...
		msg2[i] = 0x5555555555555555 // 01010101... (complemento)
	}

	res, hasContext := Majority([]Hypervector{msg1, msg2}, mask)
	if !hasContext {
		t.Fatalf("Grau 2 deveria retornar hasContext=true")
	}

	// Com msg1 ^ msg2 == 0xFFFFFFFFFFFFFFFF, todos os bits são empates (1 e 0).
	// Portanto, o resultado DEVE ser idêntico à tieBreakerMask determinística!
	if res != mask {
		t.Fatalf("Desempate determinístico em grau 2 falhou: res != mask")
	}
}

func TestMajorityDegreeOdd(t *testing.T) {
	// Com 3 mensagens, se 2 concordam, o resultado deve ser a maioria estrita sem depender da máscara
	var m1, m2, m3, mask Hypervector
	for i := 0; i < WordsPerVector; i++ {
		m1[i] = 0xFFFFFFFFFFFFFFFF
		m2[i] = 0xFFFFFFFFFFFFFFFF
		m3[i] = 0x0000000000000000
		mask[i] = 0x0000000000000000
	}

	res, _ := Majority([]Hypervector{m1, m2, m3}, mask)
	for i := 0; i < WordsPerVector; i++ {
		if res[i] != 0xFFFFFFFFFFFFFFFF {
			t.Fatalf("Majority ímpar com 2 concordantes de 3 deveria ter resultado 0xFF..")
		}
	}
}
