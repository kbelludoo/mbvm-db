package mbvmdb

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func generateBenchmarkCodebase(numStructs, numFuncs int) string {
	var sb strings.Builder
	sb.WriteString("package bench\n\n")

	for i := 0; i < numStructs; i++ {
		sb.WriteString(fmt.Sprintf("type Model%d struct {\n", i))
		sb.WriteString(fmt.Sprintf("\tID int\n\tName string\n\tRef *Model%d\n", (i+1)%numStructs))
		sb.WriteString("}\n\n")
	}

	for i := 0; i < numFuncs; i++ {
		nextCallee := (i + 1) % numFuncs
		sb.WriteString(fmt.Sprintf("func ProcessStep%d(m *Model%d) {\n", i, i%numStructs))
		sb.WriteString(fmt.Sprintf("\tProcessStep%d(nil)\n", nextCallee))
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func BenchmarkIngestGoSource(b *testing.B) {
	src := generateBenchmarkCodebase(50, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db := New()
		_ = db.IngestSource("bench.go", src)
	}
}

func BenchmarkExtractSliceDepth1(b *testing.B) {
	src := generateBenchmarkCodebase(50, 100)
	db := New()
	_ = db.IngestSource("bench.go", src)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.ExtractSlice("ProcessStep0", 1)
	}
}

func BenchmarkExtractSliceDepth2(b *testing.B) {
	src := generateBenchmarkCodebase(50, 100)
	db := New()
	_ = db.IngestSource("bench.go", src)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.ExtractSlice("ProcessStep0", 2)
	}
}

func BenchmarkExtractSliceDepth3(b *testing.B) {
	src := generateBenchmarkCodebase(50, 100)
	db := New()
	_ = db.IngestSource("bench.go", src)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.ExtractSlice("ProcessStep0", 3)
	}
}

func BenchmarkSaveToFile(b *testing.B) {
	src := generateBenchmarkCodebase(50, 100)
	db := New()
	_ = db.IngestSource("bench.go", src)

	tmpDir := b.TempDir()
	outPath := filepath.Join(tmpDir, "bench.mbvmdb")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.SaveToFile(outPath)
	}
}

func BenchmarkLoadFromFile(b *testing.B) {
	src := generateBenchmarkCodebase(50, 100)
	db := New()
	_ = db.IngestSource("bench.go", src)

	tmpDir := b.TempDir()
	outPath := filepath.Join(tmpDir, "bench.mbvmdb")
	_ = db.SaveToFile(outPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadFromFile(outPath)
	}
}

func BenchmarkProjectSlice(b *testing.B) {
	src := generateBenchmarkCodebase(50, 100)
	db := New()
	_ = db.IngestSource("bench.go", src)
	slice := db.ExtractSlice("ProcessStep0", 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.ProjectSlice(slice.Symbols)
	}
}

func BenchmarkStringPoolIntern(b *testing.B) {
	sp := NewStringPool()
	tokens := []string{"Model", "ProcessStep", "ID", "Name", "int", "string", "error"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tok := range tokens {
			_ = sp.Intern(tok)
		}
	}
}
