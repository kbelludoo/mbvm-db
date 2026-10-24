package mbvmdb

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestNeuralIndexAlignmentAndRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_neural_aligned.bin")

	totalSymbols := uint32(10)
	idx := NewNeuralIndex(totalSymbols)

	// Preenche com padrões determinísticos
	vec0 := make([]uint64, NeuralWordsPerVec)
	for i := range vec0 {
		vec0[i] = 0xAAAAAAAAAAAAAAAA
	}
	vec1 := make([]uint64, NeuralWordsPerVec)
	for i := range vec1 {
		vec1[i] = 0x5555555555555555
	}

	if err := idx.SetVector(0, vec0); err != nil {
		t.Fatalf("SetVector 0 falhou: %v", err)
	}
	if err := idx.SetVector(1, vec1); err != nil {
		t.Fatalf("SetVector 1 falhou: %v", err)
	}

	// 0xAA XOR 0x55 = 0xFF (todos os 64 bits trocados)
	// Para 128 palavras: 128 * 64 = 8192 bits trocados (ortogonalidade máxima)
	dist := idx.HammingDistance(0, 1)
	if dist != 8192 {
		t.Fatalf("Distância esperada 8192, obtida %d", dist)
	}

	// Testa distância para si mesmo (deve ser 0)
	distSelf := idx.HammingDistance(0, 0)
	if distSelf != 0 {
		t.Fatalf("Distância para si mesmo esperada 0, obtida %d", distSelf)
	}

	// Salva e valida alinhamento de 64 bytes
	if err := idx.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile falhou: %v", err)
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("os.Stat falhou: %v", err)
	}
	expectedSize := int64(NeuralHeaderSize) + int64(totalSymbols)*int64(NeuralVectorBytes)
	if stat.Size() != expectedSize {
		t.Fatalf("Tamanho de arquivo incorreto: esperado %d, obtido %d", expectedSize, stat.Size())
	}

	// Lê de volta
	loaded, err := LoadNeuralIndexFromFile(filePath)
	if err != nil {
		t.Fatalf("LoadNeuralIndexFromFile falhou: %v", err)
	}
	if loaded.TotalSymbols != totalSymbols {
		t.Fatalf("TotalSymbols incorreto: esperado %d, obtido %d", totalSymbols, loaded.TotalSymbols)
	}
	if loaded.HammingDistance(0, 1) != 8192 {
		t.Fatalf("Distância carregada incorreta: %d", loaded.HammingDistance(0, 1))
	}
}

// -------------------------------------------------------------
// BENCHMARKS OFICIAIS COMPARATIVOS SOLICITADOS PELO ZYON
// -------------------------------------------------------------

// BenchmarkHammingDistance8K mede a velocidade de comparar dois nós com 8.192 bits (128 uint64 XOR+POPCNT)
func BenchmarkHammingDistance8K(b *testing.B) {
	idx := NewNeuralIndex(2)
	v0 := make([]uint64, NeuralWordsPerVec)
	v1 := make([]uint64, NeuralWordsPerVec)
	for i := 0; i < NeuralWordsPerVec; i++ {
		v0[i] = rand.Uint64()
		v1[i] = rand.Uint64()
	}
	idx.SetVector(0, v0)
	idx.SetVector(1, v1)

	b.ResetTimer()
	b.ReportAllocs()

	var totalDist int
	for i := 0; i < b.N; i++ {
		totalDist += idx.HammingDistance(0, 1)
	}
	_ = totalDist
}

// BenchmarkCosineSimilarityFloat32_384 mede a velocidade tradicional de similaridade de cosseno em float32 (384 dimensões - MiniLM)
func BenchmarkCosineSimilarityFloat32_384(b *testing.B) {
	v0 := make([]float32, 384)
	v1 := make([]float32, 384)
	for i := 0; i < 384; i++ {
		v0[i] = rand.Float32()
		v1[i] = rand.Float32()
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalSim float32
	for i := 0; i < b.N; i++ {
		var dot, normA, normB float32
		for j := 0; j < 384; j++ {
			dot += v0[j] * v1[j]
			normA += v0[j] * v0[j]
			normB += v1[j] * v1[j]
		}
		totalSim += dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
	}
	_ = totalSim
}
