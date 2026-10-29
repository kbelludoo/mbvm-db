package erdosstraus

import (
	"strconv"
	"testing"
)

func modularBenchmarkRules(n int, scenario string) []*Rule {
	rules := make([]*Rule, n)
	for i := range rules {
		rule := &Rule{ID: "bench-" + strconv.Itoa(i)}
		switch scenario {
		case "DistinctResiduesSameMod":
			rule.Modulus = 1000003
			rule.Residues = []int64{int64(i)}
		case "AllSameResidue":
			rule.Modulus = 1000003
			rule.Residues = []int64{1}
		case "ManyDifferentModuli":
			// All moduli are in (1,002,000 .. 1,004,000),
			// hence no two distinct moduli divide one another.
			// However, finding compatible moduli still costs O(N^2)
			// when all modulus groups are unique.
			rule.Modulus = 1002001 + int64(i)
			rule.Residues = []int64{1}
		}
		rules[i] = rule
	}
	return rules
}

// BenchmarkCSRModularIndexVsPairwise uses the same independent O(N²)
// reference oracle as the exact-parity tests, in the same Go binary.
// Dense real output and many distinct moduli are reported separately.
func BenchmarkCSRModularIndexVsPairwise(b *testing.B) {
	for _, setting := range []struct{
		scenario string
		sizes []int
	}{
		{"DistinctResiduesSameMod", []int{256, 1024, 2048}},
		{"AllSameResidue", []int{64, 256}},
		{"ManyDifferentModuli", []int{256, 1024}},
	} {
		for _, n := range setting.sizes {
			rules := modularBenchmarkRules(n, setting.scenario)
			for _, implementation := range []struct{
				name string
				build func([]*Rule)*MathCSRGraph
			}{
				{"Indexed", NewMathCSRGraph},
				{"LegacyPairwise", legacyCSRPairwiseReference},
			} {
				b.Run(setting.scenario+"/N="+strconv.Itoa(n)+"/"+implementation.name,func(b *testing.B){
					b.ReportAllocs()
					var edgeCount int
					b.ResetTimer()
					for i:=0;i<b.N;i++{
						g:=implementation.build(rules)
						edgeCount+=len(g.ColIndices)
					}
					graphLayoutBenchmarkSink=uint64(edgeCount)
				})
			}
		}
	}
}
