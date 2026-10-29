package closedloop

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode"

	hdccsr "github.com/kbelludoo/mbvm-db/experiments/hdc-csr"
)

// Tokens use declaration metadata only. Function bodies, call expressions,
// identity hashes and hidden graph neighborhoods are never model features.
func tokens(s string) []string {
	var text strings.Builder
	previous := rune(0)
	for _, r := range s {
		if unicode.IsUpper(r) && unicode.IsLower(previous) {
			text.WriteByte(' ')
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			text.WriteRune(unicode.ToLower(r))
		} else {
			text.WriteByte(' ')
		}
		previous = r
	}
	seen := map[string]bool{}
	out := []string{}
	for _, t := range strings.Fields(text.String()) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func overlap(a, b []string) float64 {
	if len(a)+len(b) == 0 {
		return 0
	}
	common := 0
	for _, x := range a {
		for _, y := range b {
			if x == y {
				common++
				break
			}
		}
	}
	return float64(common) / float64(len(a)+len(b)-common)
}

func encodeSymbol(s Symbol) hdccsr.Hypervector {
	items := []string{}
	for _, t := range s.nameTokens {
		items = append(items, "name:"+t)
	}
	for _, t := range s.typeTokens {
		items = append(items, "type:"+t)
	}
	if len(items) == 0 {
		items = append(items, "empty")
	}
	var v hdccsr.Hypervector
	for w := range v {
		var counts [64]int
		for _, item := range items {
			h := sha256.Sum256([]byte(fmt.Sprintf("closed-loop-metadata-v1:%d:%s", w, item)))
			word := binary.LittleEndian.Uint64(h[:8])
			for bit := range 64 {
				counts[bit] += int((word >> bit) & 1)
			}
		}
		for bit, count := range counts {
			if count*2 > len(items) || (count*2 == len(items) && bit%2 == 0) {
				v[w] |= uint64(1) << bit
			}
		}
	}
	return v
}

func flag(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func features(a, b Symbol) [featureCount]float64 {
	prefix := len(a.nameTokens) > 0 && len(b.nameTokens) > 0 && a.nameTokens[0] == b.nameTokens[0]
	return [featureCount]float64{
		overlap(a.nameTokens, b.nameTokens), overlap(a.typeTokens, b.typeTokens),
		1 - float64(hdccsr.HammingDistance(a.vector, b.vector))/hdccsr.VectorBits,
		flag(a.Receiver != "" && a.Receiver == b.Receiver), flag(a.File == b.File), flag(prefix),
		flag(firstUpper(b.Name)), flag(firstUpper(a.Name)), flag(b.Receiver != ""), flag(a.Receiver != ""),
		overlap(a.nameTokens, b.typeTokens), overlap(a.typeTokens, b.nameTokens),
	}
}

func firstUpper(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

func lexicalScore(x [featureCount]float64) float64 {
	return 0.5*x[0] + 0.2*x[1] + 0.15*x[3] + 0.1*x[4] + 0.05*x[5]
}
