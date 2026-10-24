package hdccsr

import (
	"math/bits"
)

const (
	// VectorBits define a dimensionalidade hiperdimensional (8.192 bits = 1.024 bytes).
	VectorBits = 8192
	// WordsPerVector é o número de palavras uint64 necessárias para 8.192 bits.
	WordsPerVector = 128
)

// Hypervector representa um vetor binário denso de 8.192 bits alinhado em 128 palavras uint64.
type Hypervector [WordsPerVector]uint64

// GenerateRandomHypervector gera um hipervetor determinístico a partir de uma fonte de números pseudo-aleatórios.
func GenerateRandomHypervector(rng interface{ Uint64() uint64 }) Hypervector {
	var v Hypervector
	for i := 0; i < WordsPerVector; i++ {
		v[i] = rng.Uint64()
	}
	return v
}

// Bind executa o vínculo composicional entre dois hipervetores: A ⊕ B.
func Bind(a, b Hypervector) Hypervector {
	var out Hypervector
	for i := 0; i < WordsPerVector; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

// Unbind desfaz o vínculo: para representações binárias BIND == UNBIND (XOR).
func Unbind(bound, key Hypervector) Hypervector {
	return Bind(bound, key)
}

// RotL8K rotaciona circularmente o hipervetor de 8.192 bits para a esquerda por k posições.
func RotL8K(v Hypervector, k int) Hypervector {
	k = ((k % VectorBits) + VectorBits) % VectorBits
	if k == 0 {
		return v
	}

	wordShift := k / 64
	bitShift := uint(k % 64)

	var out Hypervector
	if bitShift == 0 {
		for i := 0; i < WordsPerVector; i++ {
			srcIdx := (i + wordShift) % WordsPerVector
			out[i] = v[srcIdx]
		}
		return out
	}

	compShift := 64 - bitShift
	for i := 0; i < WordsPerVector; i++ {
		srcIdx1 := (i + wordShift) % WordsPerVector
		srcIdx2 := (i + wordShift + 1) % WordsPerVector
		out[i] = (v[srcIdx1] >> bitShift) | (v[srcIdx2] << compShift)
	}
	return out
}

// RotR8K rotaciona circularmente o hipervetor de 8.192 bits para a direita por k posições.
func RotR8K(v Hypervector, k int) Hypervector {
	k = ((k % VectorBits) + VectorBits) % VectorBits
	return RotL8K(v, VectorBits-k)
}

// HammingDistance calcula a distância de Hamming entre dois hipervetores (0 a 8.192).
func HammingDistance(a, b Hypervector) int {
	dist := 0
	for i := 0; i < WordsPerVector; i++ {
		dist += bits.OnesCount64(a[i] ^ b[i])
	}
	return dist
}

// Majority3 calcula a maioria para 3 vetores via portas lógicas bitwise diretas:
// (A & B) | (A & C) | (B & C)
func Majority3(a, b, c Hypervector) Hypervector {
	var out Hypervector
	for i := 0; i < WordsPerVector; i++ {
		ai := a[i]
		bi := b[i]
		ci := c[i]
		out[i] = (ai & bi) | (ai & ci) | (bi & ci)
	}
	return out
}

// Majority4 calcula a maioria para 4 vetores via portas lógicas bitwise diretas com desempate determinístico:
// - Se >= 3 mensagens possuem bit 1 -> 1
// - Se exatamente 2 possuem bit 1 -> usa o bit da tieBreakerMask
// - Se <= 1 possuem bit 1 -> 0
func Majority4(a, b, c, d, tieBreaker Hypervector) Hypervector {
	var out Hypervector
	for i := 0; i < WordsPerVector; i++ {
		ai, bi, ci, di := a[i], b[i], c[i], d[i]
		tie := tieBreaker[i]

		// Condições de pelo menos 3 bits '1'
		atLeast3 := (ai & bi & ci) | (ai & bi & di) | (ai & ci & di) | (bi & ci & di)

		// Condições de pelo menos 2 bits '1' (qualquer par)
		atLeast2 := (ai & bi) | (ai & ci) | (ai & di) | (bi & ci) | (bi & di) | (ci & di)

		// Exatamente 2 bits '1' é quando temos pelo menos 2, mas não pelo menos 3
		exactly2 := atLeast2 &^ atLeast3

		out[i] = atLeast3 | (exactly2 & tie)
	}
	return out
}

// Majority agrega um conjunto arbitrário de mensagens com bitwise vectorization completa.
func Majority(messages []Hypervector, tieBreakerMask Hypervector) (Hypervector, bool) {
	n := len(messages)
	if n == 0 {
		return Hypervector{}, false
	}
	if n == 1 {
		return messages[0], true
	}
	if n == 2 {
		var out Hypervector
		m0 := messages[0]
		m1 := messages[1]
		for i := 0; i < WordsPerVector; i++ {
			bothOnes := m0[i] & m1[i]
			tieBits := m0[i] ^ m1[i]
			out[i] = bothOnes | (tieBits & tieBreakerMask[i])
		}
		return out, true
	}
	if n == 3 {
		return Majority3(messages[0], messages[1], messages[2]), true
	}
	if n == 4 {
		return Majority4(messages[0], messages[1], messages[2], messages[3], tieBreakerMask), true
	}

	// Somador paralelo bit-slice para n >= 5
	// maxBitsCount = número de bits necessários para representar n (ex: n=5..7 -> 3 bits, n=8..15 -> 4 bits, etc.)
	maxBitsCount := 0
	for tmp := n; tmp > 0; tmp >>= 1 {
		maxBitsCount++
	}

	var out Hypervector
	threshold := n / 2
	isEven := (n % 2) == 0

	// Alocação dinâmica segura dimensionada exatamente para maxBitsCount (sem estouro de buffer)
	bitsAcc := make([]uint64, maxBitsCount)

	for w := 0; w < WordsPerVector; w++ {
		for k := 0; k < maxBitsCount; k++ {
			bitsAcc[k] = 0
		}

		// Adição em árvore bit-slice: propaga o carry através das fatias de bits
		for m := 0; m < n; m++ {
			carry := messages[m][w]
			for k := 0; k < maxBitsCount && carry != 0; k++ {
				nextCarry := bitsAcc[k] & carry
				bitsAcc[k] ^= carry
				carry = nextCarry
			}
		}

		var wordResult uint64
		tieWord := tieBreakerMask[w]

		// Extrai a contagem para cada uma das 64 colunas da palavra uint64
		for b := uint(0); b < 64; b++ {
			bitMask := uint64(1) << b
			count := 0
			for k := 0; k < maxBitsCount; k++ {
				if (bitsAcc[k] & bitMask) != 0 {
					count |= (1 << k)
				}
			}

			if count > threshold {
				wordResult |= bitMask
			} else if isEven && count == threshold {
				if (tieWord & bitMask) != 0 {
					wordResult |= bitMask
				}
			}
		}
		out[w] = wordResult
	}

	return out, true
}
