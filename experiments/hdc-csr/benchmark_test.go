package hdccsr

import (
	"math/rand"
	"testing"
)

func BenchmarkHammingDistance8K(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	v1 := generateRandomHypervector(rng)
	v2 := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	sink := 0
	for i := 0; i < b.N; i++ {
		sink += HammingDistance(v1, v2)
	}
	_ = sink
}

func BenchmarkBind8K(b *testing.B) {
	rng := rand.New(rand.NewSource(2))
	v1 := generateRandomHypervector(rng)
	v2 := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	var sink Hypervector
	for i := 0; i < b.N; i++ {
		sink = Bind(v1, v2)
	}
	_ = sink
}

func BenchmarkRotL8K(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	v1 := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	var sink Hypervector
	for i := 0; i < b.N; i++ {
		sink = RotL8K(v1, 13)
	}
	_ = sink
}

func BenchmarkMajority3Vectors8K(b *testing.B) {
	rng := rand.New(rand.NewSource(4))
	msgs := []Hypervector{
		generateRandomHypervector(rng),
		generateRandomHypervector(rng),
		generateRandomHypervector(rng),
	}
	mask := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	var sink Hypervector
	for i := 0; i < b.N; i++ {
		sink, _ = Majority(msgs, mask)
	}
	_ = sink
}

func BenchmarkPropagateHop1_1000Nodes(b *testing.B) {
	const totalNodes = 1000
	rng := rand.New(rand.NewSource(5))

	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = generateRandomHypervector(rng)
	}

	// Constrói grafo esparso (média de 4 arestas por nó = 4.000 arestas)
	adj := make([][]AdjEdge, totalNodes)
	for i := 0; i < totalNodes; i++ {
		for k := 0; k < 4; k++ {
			target := uint16((i + k + 1) % totalNodes)
			adj[i] = append(adj[i], AdjEdge{Target: target, Rel: RelCall})
		}
	}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(42)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hdc.PropagateHop1(graph, orig)
	}
}
