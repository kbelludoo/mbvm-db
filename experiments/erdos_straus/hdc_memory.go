package erdosstraus

import (
	"math/bits"
	"math/rand"
)

const (
	HDCDimension = 4096 // 4096-bit binary hypervector
	HDCWords     = HDCDimension / 64
)

// Hypervector represents a 4096-bit binary high-dimensional vector.
type Hypervector [HDCWords]uint64

// HDCMemory stores rule representations in hyperdimensional vector space.
type HDCMemory struct {
	RuleVectors map[string]Hypervector
	ModulusKeys map[int64]Hypervector
	ResidueKeys map[int64]Hypervector
	Rng         *rand.Rand
}

// NewHDCMemory initializes hyperdimensional memory and basis item memories.
func NewHDCMemory(seed int64) *HDCMemory {
	rng := rand.New(rand.NewSource(seed))
	return &HDCMemory{
		RuleVectors: make(map[string]Hypervector),
		ModulusKeys: make(map[int64]Hypervector),
		ResidueKeys: make(map[int64]Hypervector),
		Rng:         rng,
	}
}

// randomHV generates a random hypervector from RNG.
func (h *HDCMemory) randomHV() Hypervector {
	var hv Hypervector
	for i := 0; i < HDCWords; i++ {
		hv[i] = h.Rng.Uint64()
	}
	return hv
}

// getModulusHV retrieves or creates a basis hypervector for a modulus.
func (h *HDCMemory) getModulusHV(mod int64) Hypervector {
	if hv, ok := h.ModulusKeys[mod]; ok {
		return hv
	}
	hv := h.randomHV()
	h.ModulusKeys[mod] = hv
	return hv
}

// getResidueHV retrieves or creates a basis hypervector for a residue.
func (h *HDCMemory) getResidueHV(res int64) Hypervector {
	if hv, ok := h.ResidueKeys[res]; ok {
		return hv
	}
	hv := h.randomHV()
	h.ResidueKeys[res] = hv
	return hv
}

// XOR computes bitwise XOR (binding operator) between two hypervectors.
func XOR(a, b Hypervector) Hypervector {
	var res Hypervector
	for i := 0; i < HDCWords; i++ {
		res[i] = a[i] ^ b[i]
	}
	return res
}

// HammingDistance calculates the bitwise difference between two hypervectors.
func HammingDistance(a, b Hypervector) int {
	dist := 0
	for i := 0; i < HDCWords; i++ {
		dist += bits.OnesCount64(a[i] ^ b[i])
	}
	return dist
}

// EncodeRule produces an HDC representation by binding Modulus and Residues.
func (h *HDCMemory) EncodeRule(r *Rule) Hypervector {
	modHV := h.getModulusHV(r.Modulus)
	if len(r.Residues) == 0 {
		return modHV
	}

	// Majority superposition over bound (Modulus XOR Residue) pairs
	counts := make([]int, HDCDimension)
	for _, res := range r.Residues {
		resHV := h.getResidueHV(res)
		bound := XOR(modHV, resHV)
		for word := 0; word < HDCWords; word++ {
			w := bound[word]
			for bit := 0; bit < 64; bit++ {
				if (w & (1 << bit)) != 0 {
					counts[word*64+bit]++
				}
			}
		}
	}

	threshold := len(r.Residues) / 2
	var bundled Hypervector
	for bitIdx := 0; bitIdx < HDCDimension; bitIdx++ {
		if counts[bitIdx] > threshold {
			word := bitIdx / 64
			bit := bitIdx % 64
			bundled[word] |= (1 << bit)
		}
	}
	h.RuleVectors[r.ID] = bundled
	return bundled
}

// QueryQuery encodes an integer n under candidate moduli and finds the closest rule in memory.
func (h *HDCMemory) QueryBestRule(n int64, candidateModuli []int64) (string, int) {
	bestID := ""
	minDist := HDCDimension + 1

	for _, mod := range candidateModuli {
		if mod <= 0 {
			continue
		}
		rem := n % mod
		modHV := h.getModulusHV(mod)
		resHV := h.getResidueHV(rem)
		queryHV := XOR(modHV, resHV)

		for ruleID, ruleHV := range h.RuleVectors {
			dist := HammingDistance(queryHV, ruleHV)
			if dist < minDist {
				minDist = dist
				bestID = ruleID
			}
		}
	}

	return bestID, minDist
}
