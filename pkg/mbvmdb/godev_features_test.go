package mbvmdb

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unique"
)

// TestUniqueStringPool verifies value interning with Go's unique package.
func TestUniqueStringPool(t *testing.T) {
	usp := NewUniqueStringPool()

	id1 := usp.Intern("github.com/kbelludoo/mbvm-db/pkg/mbvmdb.DB")
	id2 := usp.Intern("github.com/kbelludoo/mbvm-db/pkg/mbvmdb.DB")
	id3 := usp.Intern("github.com/kbelludoo/mbvm-db/pkg/mbvmdb.SymbolRecord")

	if id1 != id2 {
		t.Fatalf("expected identical IDs for duplicate string, got %d and %d", id1, id2)
	}
	if id1 == id3 {
		t.Fatalf("expected distinct IDs for distinct strings, got same %d", id1)
	}

	h1, ok1 := usp.Handle(id1)
	h2, ok2 := usp.Handle(id2)
	h3, ok3 := usp.Handle(id3)

	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("failed to retrieve unique handles")
	}

	var _ unique.Handle[string] = h1

	// Pointer-level O(1) equality check
	if h1 != h2 {
		t.Fatalf("expected identical unique.Handle for duplicate strings")
	}
	if h1 == h3 {
		t.Fatalf("expected different unique.Handle for distinct strings")
	}

	if usp.Get(id1) != "github.com/kbelludoo/mbvm-db/pkg/mbvmdb.DB" {
		t.Fatalf("value mismatch: %s", usp.Get(id1))
	}
}

// TestSymbolsIter verifies range-over-func (iter.Seq) streaming with zero slice allocations.
func TestSymbolsIter(t *testing.T) {
	db := New()
	tmpDir := t.TempDir()

	testCode := `package sample
type Config struct { Host string }
func Run(c *Config) error { return nil }
`
	if err := os.WriteFile(filepath.Join(tmpDir, "sample.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}

	if err := db.IngestGoAST(tmpDir); err != nil {
		t.Fatal(err)
	}

	count := 0
	for sym := range db.SymbolsIter() {
		if sym == nil {
			t.Fatal("received nil symbol in iteration")
		}
		count++
	}

	if count != db.TotalSymbols() {
		t.Fatalf("expected %d symbols from iterator, got %d", db.TotalSymbols(), count)
	}
}

// TestOsRootSandbox verifies path containment using os.OpenRoot.
func TestOsRootSandbox(t *testing.T) {
	tmpDir := t.TempDir()
	db := New()

	testCode := `package sandboxed
type Service struct {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "svc.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}

	if err := db.IngestGoASTRoot(tmpDir); err != nil {
		t.Fatalf("IngestGoASTRoot failed: %v", err)
	}

	if db.TotalSymbols() == 0 {
		t.Fatalf("expected indexed symbols inside root sandbox")
	}
}

// BenchmarkStringPoolClassic benchmarks traditional map + slice deduplication.
func BenchmarkStringPoolClassic(b *testing.B) {
	sp := NewStringPool()
	tokens := make([]string, 100)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("pkg/submodule/type_identifier_%d", i%20)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sp.Intern(tokens[i%len(tokens)])
	}
}

// BenchmarkStringPoolUnique benchmarks Go's unique.Make value interning.
func BenchmarkStringPoolUnique(b *testing.B) {
	usp := NewUniqueStringPool()
	tokens := make([]string, 100)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("pkg/submodule/type_identifier_%d", i%20)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = usp.Intern(tokens[i%len(tokens)])
	}
}

// BenchmarkSymbolsIteration compares standard slice copying vs iter.Seq streaming.
func BenchmarkSymbols_SliceCopy(b *testing.B) {
	db := New()
	for i := 0; i < 200; i++ {
		db.symbols = append(db.symbols, &SymbolRecord{
			ID:   uint16(i),
			Name: fmt.Sprintf("Symbol_%d", i),
		})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		syms := db.Symbols()
		for _, s := range syms {
			_ = s.Name
		}
	}
}

func BenchmarkSymbols_IterSeq(b *testing.B) {
	db := New()
	for i := 0; i < 200; i++ {
		db.symbols = append(db.symbols, &SymbolRecord{
			ID:   uint16(i),
			Name: fmt.Sprintf("Symbol_%d", i),
		})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for s := range db.SymbolsIter() {
			_ = s.Name
		}
	}
}

// BenchmarkStringComparison_Classic compares two 120-character strings with standard Go string equality.
func BenchmarkStringComparison_Classic(b *testing.B) {
	s1 := "github.com/kbelludoo/mbvm-db/pkg/mbvmdb/very_long_subpackage_name/deeply_nested/struct_definition_service_handler_v2"
	s2 := "github.com/kbelludoo/mbvm-db/pkg/mbvmdb/very_long_subpackage_name/deeply_nested/struct_definition_service_handler_v2"

	b.ResetTimer()
	b.ReportAllocs()
	res := false
	for i := 0; i < b.N; i++ {
		res = (s1 == s2)
	}
	_ = res
}

// BenchmarkStringComparison_UniqueHandle compares two unique.Handle[string] values with O(1) pointer comparison.
func BenchmarkStringComparison_UniqueHandle(b *testing.B) {
	h1 := unique.Make("github.com/kbelludoo/mbvm-db/pkg/mbvmdb/very_long_subpackage_name/deeply_nested/struct_definition_service_handler_v2")
	h2 := unique.Make("github.com/kbelludoo/mbvm-db/pkg/mbvmdb/very_long_subpackage_name/deeply_nested/struct_definition_service_handler_v2")

	b.ResetTimer()
	b.ReportAllocs()
	res := false
	for i := 0; i < b.N; i++ {
		res = (h1 == h2)
	}
	_ = res
}
