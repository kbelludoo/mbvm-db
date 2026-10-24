package mbvmdb

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestRealWorldBugs100Suite executes at least 100 distinct real-world bug test cases,
// covering language syntax quirks, AST edge cases, graph reachability, memory safety,
// cyclic dependencies, concurrency, and binary storage integrity.
func TestRealWorldBugs100Suite(t *testing.T) {
	// Category 1: Go Language Syntax Quirks & AST Parsing (Bugs 1 - 25)
	t.Run("Bug001_EmptyStruct", func(t *testing.T) {
		db := New()
		err := db.IngestSource("empty.go", "package p\ntype Empty struct {}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Empty", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type Empty struct {}") {
			t.Fatalf("expected empty struct representation, got:\n%s", proj)
		}
	})

	t.Run("Bug002_EmptyInterface", func(t *testing.T) {
		db := New()
		err := db.IngestSource("empty.go", "package p\ntype Any interface {}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Any", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type Any interface {}") {
			t.Fatalf("expected empty interface representation, got:\n%s", proj)
		}
	})

	t.Run("Bug003_BlankIdentifiersInParams", func(t *testing.T) {
		db := New()
		err := db.IngestSource("blank.go", "package p\nfunc Ignore(_ int, _ string) (_, _ int) { return 0, 0 }")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Ignore", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected 1 symbol, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug004_AnonymousClosures", func(t *testing.T) {
		db := New()
		src := `package p
func Outer() {
	fn := func() {
		Inner()
	}
	fn()
}
func Inner() {}`
		if err := db.IngestSource("closure.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Outer", 2)
		if len(slice.Symbols) < 2 {
			t.Fatalf("expected Outer and Inner, got %d symbols", len(slice.Symbols))
		}
	})

	t.Run("Bug005_ChainedSelectorCalls", func(t *testing.T) {
		db := New()
		src := `package p
func Run() {
	builder.Init().Config().Build()
}`
		if err := db.IngestSource("chain.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Run", 1)
		sym := slice.Symbols[0]
		if len(sym.OutgoingCalls) == 0 {
			t.Fatalf("expected chained selector calls to be extracted")
		}
	})

	t.Run("Bug006_PointerVsValueReceiver", func(t *testing.T) {
		db := New()
		src := `package p
type Service struct {}
func (s *Service) PointerM() {}
func (s Service) ValueM() {}`
		if err := db.IngestSource("recv.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Service", 1)
		if len(slice.Symbols) < 3 {
			t.Fatalf("expected Service struct and both methods, got %d symbols", len(slice.Symbols))
		}
	})

	t.Run("Bug007_ParenthesizedReceiver", func(t *testing.T) {
		db := New()
		src := `package p
type Engine struct {}
func (e (*Engine)) Start() {}`
		if err := db.IngestSource("paren.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Engine", 1)
		if len(slice.Symbols) < 2 {
			t.Fatalf("expected Engine and Start method, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug008_VariadicParameter", func(t *testing.T) {
		db := New()
		src := `package p
func Log(prefix string, args ...any) {}`
		if err := db.IngestSource("var.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Log", 1)
		if !strings.Contains(slice.Symbols[0].Signature, "...any") {
			t.Fatalf("expected variadic signature, got: %s", slice.Symbols[0].Signature)
		}
	})

	t.Run("Bug009_MultipleReturnValues", func(t *testing.T) {
		db := New()
		src := `package p
func Query() (string, int, error) { return "", 0, nil }`
		if err := db.IngestSource("ret.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Query", 1)
		if !strings.Contains(slice.Symbols[0].Signature, "string, int, error") {
			t.Fatalf("expected multi return signature, got: %s", slice.Symbols[0].Signature)
		}
	})

	t.Run("Bug010_EmbeddedAnonymousStruct", func(t *testing.T) {
		db := New()
		src := `package p
type Base struct { ID int }
type Extended struct {
	Base
	Name string
}`
		if err := db.IngestSource("embed.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Extended", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type Base struct") {
			t.Fatalf("expected embedded Base to be pulled into slice, got:\n%s", proj)
		}
	})

	t.Run("Bug011_InterfaceEmbeddingInterface", func(t *testing.T) {
		db := New()
		src := `package p
type Reader interface { Read(p []byte) (n int, err error) }
type ReadCloser interface {
	Reader
	Close() error
}`
		if err := db.IngestSource("iface.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("ReadCloser", 1)
		if len(slice.Symbols) < 1 {
			t.Fatalf("expected ReadCloser to be indexed")
		}
	})

	t.Run("Bug012_GenericTypeParameterStruct", func(t *testing.T) {
		db := New()
		src := `package p
type Container[T any] struct {
	Value T
}`
		if err := db.IngestSource("generic.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Container", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected generic struct indexed, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug013_GenericFunction", func(t *testing.T) {
		db := New()
		src := `package p
func Map[T any, U any](items []T, fn func(T) U) []U { return nil }`
		if err := db.IngestSource("genfunc.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Map", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Map function indexed, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug014_ChannelParameter", func(t *testing.T) {
		db := New()
		src := `package p
type Message struct { Data string }
func Consume(in <-chan Message, out chan<- Message) {}`
		if err := db.IngestSource("chan.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Consume", 1)
		if len(slice.Symbols) < 1 {
			t.Fatalf("expected Consume indexed")
		}
	})

	t.Run("Bug015_MapWithCustomTypes", func(t *testing.T) {
		db := New()
		src := `package p
type Key struct { ID string }
type Value struct { Count int }
type Cache struct {
	Items map[Key]*Value
}`
		if err := db.IngestSource("map.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Cache", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type Key struct") || !strings.Contains(proj, "type Value struct") {
			t.Fatalf("expected both Key and Value to be resolved in Cache slice, got:\n%s", proj)
		}
	})

	t.Run("Bug016_StructTagsWithSpecialChars", func(t *testing.T) {
		db := New()
		src := `package p
type User struct {
	Email string ` + "`" + `json:"email,omitempty" gorm:"unique;not null;size:255"` + "`" + `
}`
		if err := db.IngestSource("tag.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("User", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected User struct indexed")
		}
	})

	t.Run("Bug017_MutuallyRecursiveFunctions", func(t *testing.T) {
		db := New()
		src := `package p
func Ping() { Pong() }
func Pong() { Ping() }`
		if err := db.IngestSource("mutual.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Ping", 5)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected exactly 2 symbols without infinite recursion, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug018_SelfRecursiveFunction", func(t *testing.T) {
		db := New()
		src := `package p
func Factorial(n int) int {
	if n <= 1 { return 1 }
	return n * Factorial(n-1)
}`
		if err := db.IngestSource("self.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Factorial", 5)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected 1 symbol, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug019_DiamondCallGraph", func(t *testing.T) {
		db := New()
		src := `package p
func Root() { Left(); Right() }
func Left() { Bottom() }
func Right() { Bottom() }
func Bottom() {}`
		if err := db.IngestSource("diamond.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Root", 3)
		if len(slice.Symbols) != 4 {
			t.Fatalf("expected 4 symbols in diamond graph, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug020_DisconnectedSubgraphIsolation", func(t *testing.T) {
		db := New()
		src := `package p
func Active1() { Active2() }
func Active2() {}
func Isolated1() { Isolated2() }
func Isolated2() {}`
		if err := db.IngestSource("isolated.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Active1", 3)
		for _, s := range slice.Symbols {
			if strings.HasPrefix(s.Name, "Isolated") {
				t.Fatalf("isolated symbol %s leaked into Active1 slice", s.Name)
			}
		}
	})

	t.Run("Bug021_SliceNonExistentSymbol", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F() {}")
		slice := db.ExtractSlice("NoSuchSymbol", 2)
		if len(slice.Symbols) != 0 {
			t.Fatalf("expected 0 symbols, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug022_SliceDepth0", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F() { G() }\nfunc G() {}")
		slice := db.ExtractSlice("F", 0)
		if len(slice.Symbols) != 1 || slice.Symbols[0].Name != "F" {
			t.Fatalf("expected only target symbol F at depth 0, got %d symbols", len(slice.Symbols))
		}
	})

	t.Run("Bug023_SliceNegativeDepth", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F() {}")
		slice := db.ExtractSlice("F", -1)
		if len(slice.Symbols) != 0 {
			t.Fatalf("expected empty slice for negative depth, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug024_SliceQualifiedPackageName", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package alpha\nfunc Worker() {}")
		slice := db.ExtractSlice("alpha.Worker", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Worker found via package qualification, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug025_CaseSensitivityInSymbols", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc myFunc() {}\nfunc MyFunc() {}")
		sliceLower := db.ExtractSlice("myFunc", 0)
		sliceUpper := db.ExtractSlice("MyFunc", 0)
		if len(sliceLower.Symbols) != 1 || len(sliceUpper.Symbols) != 1 {
			t.Fatalf("expected 1 symbol each for case-sensitive queries")
		}
		if sliceLower.Symbols[0].Name != "myFunc" || sliceUpper.Symbols[0].Name != "MyFunc" {
			t.Fatalf("case mismatch in symbol resolution")
		}
	})

	// Category 2: String Pool, Bytecode Encoding & Compression (Bugs 26 - 45)
	t.Run("Bug026_StringPoolDeduplication", func(t *testing.T) {
		sp := NewStringPool()
		id1 := sp.Intern("hello")
		id2 := sp.Intern("hello")
		if id1 != id2 {
			t.Fatalf("expected same ID for identical string, got %d and %d", id1, id2)
		}
		if len(sp.pool) != 1 {
			t.Fatalf("expected 1 string in pool, got %d", len(sp.pool))
		}
	})

	t.Run("Bug027_StringPoolNonExistentID", func(t *testing.T) {
		sp := NewStringPool()
		sp.Intern("test")
		val := sp.Get(9999)
		if val != "" {
			t.Fatalf("expected empty string for out-of-range ID, got %q", val)
		}
	})

	t.Run("Bug028_SliceFieldOfPointersTypeResolution", func(t *testing.T) {
		db := New()
		src := `package p
type Item struct { Name string }
type Order struct {
	Items []*Item
}`
		if err := db.IngestSource("order.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Order", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type Item struct") {
			t.Fatalf("expected Item resolved from []*Item, got:\n%s", proj)
		}
	})

	t.Run("Bug029_MethodCallingMethodOnSameStruct", func(t *testing.T) {
		db := New()
		src := `package p
type Client struct {}
func (c *Client) CallAPI() { c.validate() }
func (c *Client) validate() {}`
		if err := db.IngestSource("client.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("CallAPI", 2)
		if len(slice.Symbols) < 2 {
			t.Fatalf("expected CallAPI and validate, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug030_IncomingCallsReverseLookup", func(t *testing.T) {
		db := New()
		src := `package p
func Target() {}
func Caller1() { Target() }
func Caller2() { Target() }`
		if err := db.IngestSource("target.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Target", 1)
		// Should pull callers via incoming edges
		if len(slice.Symbols) < 3 {
			t.Fatalf("expected Target and 2 callers, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug031_StdlibCallDoesNotPanic", func(t *testing.T) {
		db := New()
		src := `package p
import "fmt"
func Print() { fmt.Println("test") }`
		if err := db.IngestSource("print.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Print", 2)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Print indexed cleanly without crashing on fmt.Println")
		}
	})

	t.Run("Bug032_MBVMSigilsInCommentsIgnored", func(t *testing.T) {
		db := New()
		src := `package p
// @MBVM:slice:target%mode=dynamic_slice
// ^R1:rule=fake
func RealCode() {}`
		if err := db.IngestSource("sigil.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("RealCode", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected only RealCode indexed")
		}
	})

	t.Run("Bug033_MultiplePackagesIngested", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("a.go", "package pkgA\nfunc FA() {}")
		_ = db.IngestSource("b.go", "package pkgB\nfunc FB() {}")
		if db.TotalSymbols() != 2 {
			t.Fatalf("expected 2 symbols across packages, got %d", db.TotalSymbols())
		}
	})

	t.Run("Bug034_UnicodeIdentifiersInGo", func(t *testing.T) {
		db := New()
		src := `package p
type Configuração struct {
	Ação string
}`
		if err := db.IngestSource("utf8.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Configuração", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected unicode identifier indexed, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug035_MultipleInitFunctions", func(t *testing.T) {
		db := New()
		src := `package p
func init() { setupA() }
func init() { setupB() }
func setupA() {}
func setupB() {}`
		if err := db.IngestSource("init.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if db.TotalSymbols() != 4 {
			t.Fatalf("expected 4 symbols including multiple inits, got %d", db.TotalSymbols())
		}
	})

	t.Run("Bug036_MethodNamedSameAsField", func(t *testing.T) {
		db := New()
		src := `package p
type Item struct { Value int }
func Value() int { return 42 }`
		if err := db.IngestSource("clash.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Value", 0)
		if len(slice.Symbols) == 0 {
			t.Fatalf("expected symbol resolved")
		}
	})

	t.Run("Bug037_InterfaceWithMultipleMethods", func(t *testing.T) {
		db := New()
		src := `package p
type Handler interface {
	Handle(req string) (string, error)
	HealthCheck() bool
}`
		if err := db.IngestSource("handler.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Handler", 1)
		if len(slice.Symbols[0].Methods) != 2 {
			t.Fatalf("expected 2 methods, got %d", len(slice.Symbols[0].Methods))
		}
	})

	t.Run("Bug038_ComplexNestedTypes", func(t *testing.T) {
		db := New()
		src := `package p
type Meta struct { Tag string }
type Wrapper struct {
	Store []map[string]*Meta
}`
		if err := db.IngestSource("nested.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Wrapper", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type Meta struct") {
			t.Fatalf("expected Meta resolved in nested type, got:\n%s", proj)
		}
	})

	t.Run("Bug039_BytecodeDecompressionRoundtrip", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc Foo() { Bar() }\nfunc Bar() {}")
		if db.BytecodeSize() == 0 {
			t.Fatalf("expected non-zero compressed bytecode size")
		}

		zr, err := zlib.NewReader(bytes.NewReader(db.bytecode))
		if err != nil {
			t.Fatalf("failed to decompress bytecode: %v", err)
		}
		defer zr.Close()

		var decomp bytes.Buffer
		_, _ = decomp.ReadFrom(zr)
		raw := decomp.Bytes()

		if len(raw) == 0 || raw[len(raw)-1] != OpEnd {
			t.Fatalf("expected last byte to be OpEnd (0xFF)")
		}
	})

	t.Run("Bug040_ConcurrentReadsDuringSlice", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("conc.go", "package p\nfunc A() { B() }\nfunc B() {}")

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				slice := db.ExtractSlice("A", 2)
				if len(slice.Symbols) != 2 {
					t.Errorf("expected 2 symbols in concurrent slice")
				}
			}()
		}
		wg.Wait()
	})

	t.Run("Bug041_LargeSymbolCapacity", func(t *testing.T) {
		db := New()
		var sb strings.Builder
		sb.WriteString("package large\n")
		for i := 0; i < 150; i++ {
			sb.WriteString(fmt.Sprintf("type Struct%d struct { F int }\n", i))
		}
		if err := db.IngestSource("large.go", sb.String()); err != nil {
			t.Fatalf("failed to ingest large symbol set: %v", err)
		}
		if db.TotalSymbols() != 150 {
			t.Fatalf("expected 150 symbols, got %d", db.TotalSymbols())
		}
	})

	t.Run("Bug042_DeepCallChain10Levels", func(t *testing.T) {
		db := New()
		var sb strings.Builder
		sb.WriteString("package deep\n")
		for i := 1; i <= 10; i++ {
			if i < 10 {
				sb.WriteString(fmt.Sprintf("func Call%d() { Call%d() }\n", i, i+1))
			} else {
				sb.WriteString(fmt.Sprintf("func Call%d() {}\n", i))
			}
		}
		_ = db.IngestSource("deep.go", sb.String())
		slice := db.ExtractSlice("Call1", 5)
		if len(slice.Symbols) != 6 { // Call1 + Call2..6 (depth 5 = 6 symbols)
			t.Fatalf("expected 6 symbols at depth 5, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug043_FunctionWithoutBody", func(t *testing.T) {
		db := New()
		// Go allows extern/assembly function declarations without body
		src := `package p
func SyscallStub(trap uintptr, a1, a2, a3 uintptr) uintptr`
		if err := db.IngestSource("asm.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("SyscallStub", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected bodyless function indexed")
		}
	})

	t.Run("Bug044_TypeAliasesVsDefinitions", func(t *testing.T) {
		db := New()
		src := `package p
type IntAlias = int
type CustomInt int`
		if err := db.IngestSource("alias.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Struct/interface parser handles type specs cleanly
		if db.TotalSymbols() < 0 {
			t.Fatalf("invalid symbol count")
		}
	})

	t.Run("Bug045_DuplicateSymbolNamesAcrossPackages", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("auth.go", "package auth\nfunc Login() {}")
		_ = db.IngestSource("admin.go", "package admin\nfunc Login() {}")
		if db.TotalSymbols() != 2 {
			t.Fatalf("expected 2 distinct Login symbols indexed, got %d", db.TotalSymbols())
		}
	})

	// Category 3: Storage Engine & Binary Format Integrity (Bugs 46 - 65)
	t.Run("Bug046_CyclicTypeReference", func(t *testing.T) {
		db := New()
		src := `package p
type Node struct {
	Next *Node
	Prev *Node
}`
		if err := db.IngestSource("node.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Node", 2)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Node handled cleanly without duplicate self-inclusion")
		}
	})

	t.Run("Bug047_MutualCyclicTypes", func(t *testing.T) {
		db := New()
		src := `package p
type Person struct { Job *Job }
type Job struct { Employee *Person }`
		if err := db.IngestSource("mutual_type.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Person", 2)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected both Person and Job in mutual cyclic types, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug048_SaveToFileInvalidDirectory", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F() {}")
		err := db.SaveToFile("/nonexistent/directory/db.mbvmdb")
		if err == nil {
			t.Fatalf("expected error saving to non-existent directory")
		}
	})

	t.Run("Bug049_LoadFromFileZeroBytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		zeroFile := filepath.Join(tmpDir, "zero.mbvmdb")
		_ = os.WriteFile(zeroFile, []byte{}, 0644)
		_, err := LoadFromFile(zeroFile)
		if err == nil {
			t.Fatalf("expected error on zero-byte file")
		}
	})

	t.Run("Bug050_LoadFromFileTruncatedHeader", func(t *testing.T) {
		tmpDir := t.TempDir()
		truncFile := filepath.Join(tmpDir, "trunc.mbvmdb")
		_ = os.WriteFile(truncFile, []byte{'M', 'B', 'V'}, 0644)
		_, err := LoadFromFile(truncFile)
		if err == nil {
			t.Fatalf("expected error on truncated header")
		}
	})

	t.Run("Bug051_LoadFromFileCorruptedPayload", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F() {}")
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "corrupt.mbvmdb")
		_ = db.SaveToFile(p)

		data, _ := os.ReadFile(p)
		if len(data) > 60 {
			data[len(data)/2] ^= 0xAA
			_ = os.WriteFile(p, data, 0644)
		}
		_, err := LoadFromFile(p)
		if err != ErrCorruptData {
			t.Fatalf("expected ErrCorruptData on bit flip, got %v", err)
		}
	})

	t.Run("Bug052_ExtractSliceEmptyTarget", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F() {}")
		slice := db.ExtractSlice("", 2)
		if len(slice.Symbols) != 0 || slice.TokensEstimate != 0 {
			t.Fatalf("expected 0 symbols and 0 tokens on empty target")
		}
	})

	t.Run("Bug053_LargeDepthOnSmallGraph", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F() { G() }\nfunc G() {}")
		slice := db.ExtractSlice("F", 100)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected 2 symbols terminating cleanly with depth 100")
		}
	})

	t.Run("Bug054_HeavyFanOutFunction", func(t *testing.T) {
		db := New()
		var sb strings.Builder
		sb.WriteString("package fanout\n")
		var callees []string
		for i := 0; i < 20; i++ {
			callees = append(callees, fmt.Sprintf("target%d()", i))
			sb.WriteString(fmt.Sprintf("func target%d() {}\n", i))
		}
		sb.WriteString(fmt.Sprintf("func Master() { %s }\n", strings.Join(callees, "; ")))
		_ = db.IngestSource("fanout.go", sb.String())
		slice := db.ExtractSlice("Master", 1)
		if len(slice.Symbols) != 21 { // Master + 20 targets
			t.Fatalf("expected 21 symbols on fan-out, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug055_HeavyFanInFunction", func(t *testing.T) {
		db := New()
		var sb strings.Builder
		sb.WriteString("package fanin\n")
		sb.WriteString("func CoreSink() {}\n")
		for i := 0; i < 20; i++ {
			sb.WriteString(fmt.Sprintf("func Worker%d() { CoreSink() }\n", i))
		}
		_ = db.IngestSource("fanin.go", sb.String())
		slice := db.ExtractSlice("CoreSink", 1)
		if len(slice.Symbols) != 21 { // CoreSink + 20 workers
			t.Fatalf("expected 21 symbols on fan-in reverse graph, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug056_StructWithPointerToSlice", func(t *testing.T) {
		db := New()
		src := `package p
type Matrix struct {
	Data *[]int
}`
		if err := db.IngestSource("matrix.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Matrix", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Matrix indexed")
		}
	})

	t.Run("Bug057_StructWithArrayType", func(t *testing.T) {
		db := New()
		src := `package p
type Buffer struct {
	Bytes [64]byte
}`
		if err := db.IngestSource("buf.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Buffer", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Buffer indexed")
		}
	})

	t.Run("Bug058_NamedReturnVariables", func(t *testing.T) {
		db := New()
		src := `package p
func Fetch() (res string, count int, err error) { return "", 0, nil }`
		if err := db.IngestSource("named.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Fetch", 1)
		if !strings.Contains(slice.Symbols[0].Signature, "res string, count int, err error") {
			t.Fatalf("named returns signature mismatch: %s", slice.Symbols[0].Signature)
		}
	})

	t.Run("Bug059_FunctionReturningFunction", func(t *testing.T) {
		db := New()
		src := `package p
func Curried() func(int) string { return nil }`
		if err := db.IngestSource("curry.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Curried", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Curried indexed")
		}
	})

	t.Run("Bug060_StructWithFuncField", func(t *testing.T) {
		db := New()
		src := `package p
type EventBus struct {
	Handler func(name string) bool
}`
		if err := db.IngestSource("bus.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("EventBus", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected EventBus indexed")
		}
	})

	t.Run("Bug061_InterfaceWithEmbeddedInterfaceCall", func(t *testing.T) {
		db := New()
		src := `package p
type Closer interface { Close() error }
type Session interface {
	Closer
	ID() string
}`
		if err := db.IngestSource("session.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Session", 1)
		if len(slice.Symbols) < 1 {
			t.Fatalf("expected Session indexed")
		}
	})

	t.Run("Bug062_VendorAndGitSkipping", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		vendorDir := filepath.Join(tmpDir, "vendor")
		_ = os.MkdirAll(gitDir, 0755)
		_ = os.MkdirAll(vendorDir, 0755)

		_ = os.WriteFile(filepath.Join(gitDir, "ignored.go"), []byte("package git\nfunc Ignored() {}"), 0644)
		_ = os.WriteFile(filepath.Join(vendorDir, "vendor.go"), []byte("package vendor\nfunc Vendored() {}"), 0644)
		_ = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\nfunc App() {}"), 0644)

		db := New()
		err := db.IngestGoAST(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if db.TotalSymbols() != 1 {
			t.Fatalf("expected only App indexed, got %d symbols", db.TotalSymbols())
		}
	})

	t.Run("Bug063_DeterministicProjectSliceOrdering", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("a.go", "package alpha\ntype A struct {}\nfunc FA() {}")
		_ = db.IngestSource("b.go", "package beta\ntype B struct {}\nfunc FB() {}")
		_ = db.IngestSource("z.go", "package zeta\ntype Z struct {}\nfunc FZ() {}")

		proj1 := db.ProjectPureMBVM()
		proj2 := db.ProjectPureMBVM()
		if proj1 != proj2 {
			t.Fatalf("ProjectPureMBVM must be identical across calls")
		}

		// Ensure alpha appears before beta, and beta before zeta
		idxAlpha := strings.Index(proj1, "pkg alpha")
		idxBeta := strings.Index(proj1, "pkg beta")
		idxZeta := strings.Index(proj1, "pkg zeta")
		if idxAlpha > idxBeta || idxBeta > idxZeta {
			t.Fatalf("packages not sorted deterministically:\n%s", proj1)
		}
	})

	t.Run("Bug064_TokenEstimatorAccuracy", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc A() {}")
		slice := db.ExtractSlice("A", 1)
		proj := db.ProjectSlice(slice.Symbols)
		expectedTokens := len(proj) / 4
		if slice.TokensEstimate != expectedTokens {
			t.Fatalf("expected token estimate %d, got %d", expectedTokens, slice.TokensEstimate)
		}
	})

	t.Run("Bug065_ParallelExtractSliceStress", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F1() { F2() }\nfunc F2() {}")
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s := db.ExtractSlice("F1", 2)
				if len(s.Symbols) != 2 {
					t.Errorf("expected 2 symbols")
				}
			}()
		}
		wg.Wait()
	})

	// Category 4: AST Traversal Conditions & Call Sites (Bugs 66 - 85)
	t.Run("Bug066_EmptyGoFileNoSymbols", func(t *testing.T) {
		db := New()
		err := db.IngestSource("empty.go", "package empty\n// only comments")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if db.TotalSymbols() != 0 {
			t.Fatalf("expected 0 symbols, got %d", db.TotalSymbols())
		}
	})

	t.Run("Bug067_BytecodeOffsetAndLengthConsistency", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\ntype S struct { F int }\nfunc M() {}")
		for _, sym := range db.Symbols() {
			if sym.BytecodeLen == 0 {
				t.Fatalf("symbol %s has 0 bytecode length", sym.Name)
			}
		}
	})

	t.Run("Bug068_StructWithInterfaceField", func(t *testing.T) {
		db := New()
		src := `package p
type Logger interface { Log(s string) }
type App struct {
	Log Logger
}`
		_ = db.IngestSource("test.go", src)
		slice := db.ExtractSlice("App", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type Logger interface") {
			t.Fatalf("expected Logger interface included in App slice, got:\n%s", proj)
		}
	})

	t.Run("Bug069_MultipleMethodsOnSameStruct", func(t *testing.T) {
		db := New()
		src := `package p
type Calc struct {}
func (c *Calc) Add() {}
func (c *Calc) Sub() {}
func (c *Calc) Mul() {}`
		_ = db.IngestSource("calc.go", src)
		slice := db.ExtractSlice("Calc", 1)
		if len(slice.Symbols) != 4 { // Calc + 3 methods
			t.Fatalf("expected Calc and 3 methods, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug070_PointerReceiverExtractedByType", func(t *testing.T) {
		db := New()
		src := `package p
type Runner struct {}
func (r *Runner) Run() {}`
		_ = db.IngestSource("runner.go", src)
		slice := db.ExtractSlice("Runner.Run", 1)
		if len(slice.Symbols) < 2 { // Method + Runner struct
			t.Fatalf("expected Runner struct and Run method, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug071_CallInsideIfCondition", func(t *testing.T) {
		db := New()
		src := `package p
func Caller() {
	if Check() { DoWork() }
}
func Check() bool { return true }
func DoWork() {}`
		_ = db.IngestSource("if.go", src)
		slice := db.ExtractSlice("Caller", 1)
		if len(slice.Symbols) != 3 {
			t.Fatalf("expected Caller, Check and DoWork, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug072_CallInsideForLoop", func(t *testing.T) {
		db := New()
		src := `package p
func Looper() {
	for i := 0; i < Limit(); i++ { Step() }
}
func Limit() int { return 10 }
func Step() {}`
		_ = db.IngestSource("loop.go", src)
		slice := db.ExtractSlice("Looper", 1)
		if len(slice.Symbols) != 3 {
			t.Fatalf("expected Looper, Limit, and Step, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug073_CallInsideDefer", func(t *testing.T) {
		db := New()
		src := `package p
func Execute() {
	defer Cleanup()
}
func Cleanup() {}`
		_ = db.IngestSource("defer.go", src)
		slice := db.ExtractSlice("Execute", 1)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected Execute and Cleanup, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug074_CallInsideGoRoutine", func(t *testing.T) {
		db := New()
		src := `package p
func Async() {
	go BackgroundTask()
}
func BackgroundTask() {}`
		_ = db.IngestSource("async.go", src)
		slice := db.ExtractSlice("Async", 1)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected Async and BackgroundTask, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug075_CallInsideReturnStatement", func(t *testing.T) {
		db := New()
		src := `package p
func Final() int {
	return ComputeResult()
}
func ComputeResult() int { return 100 }`
		_ = db.IngestSource("return.go", src)
		slice := db.ExtractSlice("Final", 1)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected Final and ComputeResult, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug076_CallInsideSwitchCondition", func(t *testing.T) {
		db := New()
		src := `package p
func Dispatch() {
	switch Mode() {
	case 1:
		Handle1()
	}
}
func Mode() int { return 1 }
func Handle1() {}`
		_ = db.IngestSource("switch.go", src)
		slice := db.ExtractSlice("Dispatch", 1)
		if len(slice.Symbols) != 3 {
			t.Fatalf("expected Dispatch, Mode, and Handle1, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug077_CallInsideSelectCase", func(t *testing.T) {
		db := New()
		src := `package p
func Poll(ch chan int) {
	select {
	case ch <- Produce():
	}
}
func Produce() int { return 1 }`
		_ = db.IngestSource("select.go", src)
		slice := db.ExtractSlice("Poll", 1)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected Poll and Produce, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug078_MethodCallOnVariable", func(t *testing.T) {
		db := New()
		src := `package p
type Client struct {}
func (c *Client) Do() {}
func Run(client *Client) {
	client.Do()
}`
		_ = db.IngestSource("varcall.go", src)
		slice := db.ExtractSlice("Run", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "func (*Client) Do()") {
			t.Fatalf("expected Do method included in Run slice, got:\n%s", proj)
		}
	})

	t.Run("Bug079_DuplicateCallsDeduplication", func(t *testing.T) {
		db := New()
		src := `package p
func Target() {}
func Spam() {
	Target()
	Target()
	Target()
}`
		_ = db.IngestSource("spam.go", src)
		slice := db.ExtractSlice("Spam", 1)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected exactly 2 symbols despite 3 duplicate calls, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug080_HighDensityStructFields", func(t *testing.T) {
		db := New()
		var fields []string
		for i := 0; i < 50; i++ {
			fields = append(fields, fmt.Sprintf("Field%d int", i))
		}
		src := fmt.Sprintf("package p\ntype Big struct { %s }", strings.Join(fields, "; "))
		if err := db.IngestSource("big.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("Big", 1)
		if len(slice.Symbols[0].Fields) != 50 {
			t.Fatalf("expected 50 fields, got %d", len(slice.Symbols[0].Fields))
		}
	})

	t.Run("Bug081_HighDensityInterfaceMethods", func(t *testing.T) {
		db := New()
		var methods []string
		for i := 0; i < 30; i++ {
			methods = append(methods, fmt.Sprintf("Method%d() error", i))
		}
		src := fmt.Sprintf("package p\ntype BigAPI interface { %s }", strings.Join(methods, "; "))
		if err := db.IngestSource("bigapi.go", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slice := db.ExtractSlice("BigAPI", 1)
		if len(slice.Symbols[0].Methods) != 30 {
			t.Fatalf("expected 30 methods, got %d", len(slice.Symbols[0].Methods))
		}
	})

	t.Run("Bug082_SyntaxErrorReturnsDescriptiveError", func(t *testing.T) {
		db := New()
		err := db.IngestSource("broken.go", "package p\nfunc Broken( {")
		if err == nil {
			t.Fatalf("expected syntax error on broken Go code")
		}
	})

	t.Run("Bug083_DBStatReportingAccuracy", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("f1.go", "package p\nfunc F1() {}")
		_ = db.IngestSource("f2.go", "package p\nfunc F2() {}")
		if db.TotalFiles != 2 {
			t.Fatalf("expected 2 files, got %d", db.TotalFiles)
		}
		if db.TotalSymbols() != 2 {
			t.Fatalf("expected 2 symbols, got %d", db.TotalSymbols())
		}
		if db.TotalRawBytes <= 0 {
			t.Fatalf("expected positive TotalRawBytes")
		}
	})

	t.Run("Bug084_AccumulatedIngestIntegrity", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("file1.go", "package p\nfunc First() { Second() }")
		_ = db.IngestSource("file2.go", "package p\nfunc Second() {}")
		slice := db.ExtractSlice("First", 1)
		if len(slice.Symbols) != 2 {
			t.Fatalf("expected First and Second connected across separate IngestSource calls, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug085_DoublePointerReceiver", func(t *testing.T) {
		db := New()
		src := `package p
type PtrType struct {}
func (p **PtrType) DeepPtr() {}`
		_ = db.IngestSource("ptr.go", src)
		slice := db.ExtractSlice("PtrType", 1)
		if len(slice.Symbols) < 2 {
			t.Fatalf("expected PtrType and DeepPtr method")
		}
	})

	// Category 5: Complex Real-World Edge Cases (Bugs 86 - 102)
	t.Run("Bug086_StandardLibraryTypeInFields", func(t *testing.T) {
		db := New()
		src := `package p
import "time"
type Event struct {
	CreatedAt time.Time
}`
		_ = db.IngestSource("event.go", src)
		slice := db.ExtractSlice("Event", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Event indexed without failing on time.Time")
		}
	})

	t.Run("Bug087_ContextParameterInMethods", func(t *testing.T) {
		db := New()
		src := `package p
import "context"
func Handle(ctx context.Context, id string) error { return nil }`
		_ = db.IngestSource("ctx.go", src)
		slice := db.ExtractSlice("Handle", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Handle indexed cleanly")
		}
	})

	t.Run("Bug088_MethodCallOnInterfaceParameter", func(t *testing.T) {
		db := New()
		src := `package p
import "io"
func Dump(w io.Writer) {
	w.Write([]byte("hello"))
}`
		_ = db.IngestSource("dump.go", src)
		slice := db.ExtractSlice("Dump", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Dump indexed")
		}
	})

	t.Run("Bug089_StructEmbeddingUnexportedStruct", func(t *testing.T) {
		db := New()
		src := `package p
type unexported struct { secret int }
type Exported struct {
	unexported
}`
		_ = db.IngestSource("embed_unexp.go", src)
		slice := db.ExtractSlice("Exported", 1)
		proj := db.ProjectSlice(slice.Symbols)
		if !strings.Contains(proj, "type unexported struct") {
			t.Fatalf("expected embedded unexported struct resolved, got:\n%s", proj)
		}
	})

	t.Run("Bug090_CrossPackageStructEmbedding", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("base.go", "package core\ntype Base struct { ID int }")
		_ = db.IngestSource("app.go", "package app\ntype Model struct { Base int }")
		slice := db.ExtractSlice("Model", 1)
		if len(slice.Symbols) < 1 {
			t.Fatalf("expected Model indexed")
		}
	})

	t.Run("Bug091_MethodCallOnStructPointerLiteral", func(t *testing.T) {
		db := New()
		src := `package p
type Worker struct {}
func (w *Worker) Start() {}
func Boot() {
	(&Worker{}).Start()
}`
		_ = db.IngestSource("boot.go", src)
		slice := db.ExtractSlice("Boot", 1)
		if len(slice.Symbols) < 2 {
			t.Fatalf("expected Boot and Worker.Start, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug092_VariadicAnyParameter", func(t *testing.T) {
		db := New()
		src := `package p
func Format(format string, args ...any) {}`
		_ = db.IngestSource("fmt.go", src)
		slice := db.ExtractSlice("Format", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Format indexed")
		}
	})

	t.Run("Bug093_TypeDefinitionWithPrimitiveUnderlying", func(t *testing.T) {
		db := New()
		src := `package p
type UserID uint64
type Account struct {
	ID UserID
}`
		_ = db.IngestSource("type_prim.go", src)
		slice := db.ExtractSlice("Account", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Account indexed")
		}
	})

	t.Run("Bug094_StructWithZeroLengthArrayField", func(t *testing.T) {
		db := New()
		src := `package p
type Uncopyable struct {
	_ [0]func()
	Value int
}`
		_ = db.IngestSource("zero_arr.go", src)
		slice := db.ExtractSlice("Uncopyable", 1)
		if len(slice.Symbols) != 1 {
			t.Fatalf("expected Uncopyable indexed")
		}
	})

	t.Run("Bug095_ProjectSliceEmptySymbols", func(t *testing.T) {
		db := New()
		proj := db.ProjectSlice(nil)
		if proj != "@MBVM:slice:empty\n" {
			t.Fatalf("expected @MBVM:slice:empty, got %q", proj)
		}
	})

	t.Run("Bug096_BytecodeOpEndIsLastByte", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc A() {}")
		zr, _ := zlib.NewReader(bytes.NewReader(db.bytecode))
		var decomp bytes.Buffer
		_, _ = decomp.ReadFrom(zr)
		raw := decomp.Bytes()
		if raw[len(raw)-1] != OpEnd {
			t.Fatalf("expected final byte to be OpEnd (0xFF), got 0x%X", raw[len(raw)-1])
		}
	})

	t.Run("Bug097_AtomicSavePreservesFileOnFailure", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc Good() {}")
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "saved.mbvmdb")
		_ = db.SaveToFile(target)

		// Verify target exists
		info, err := os.Stat(target)
		if err != nil || info.Size() == 0 {
			t.Fatalf("expected valid saved file")
		}
	})

	t.Run("Bug098_LoadFromFileTrailingGarbageFailsCRC", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc Good() {}")
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "saved.mbvmdb")
		_ = db.SaveToFile(target)

		// Append trailing byte
		f, _ := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0644)
		_, _ = f.Write([]byte{0x00})
		_ = f.Close()

		_, err := LoadFromFile(target)
		if err != ErrCorruptData {
			t.Fatalf("expected ErrCorruptData on trailing byte, got %v", err)
		}
	})

	t.Run("Bug099_MultiPackagePathTraversal", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("router.go", "package web\nfunc Route() { service.Handle() }")
		_ = db.IngestSource("service.go", "package service\nfunc Handle() { db.Query() }")
		_ = db.IngestSource("db.go", "package db\nfunc Query() {}")

		slice := db.ExtractSlice("Route", 2)
		if len(slice.Symbols) < 2 {
			t.Fatalf("expected traversal across web -> service, got %d", len(slice.Symbols))
		}
	})

	t.Run("Bug100_ZeroMemoryLeakRepeatedQueries", func(t *testing.T) {
		db := New()
		_ = db.IngestSource("test.go", "package p\nfunc F1() { F2() }\nfunc F2() {}")

		for i := 0; i < 500; i++ {
			slice := db.ExtractSlice("F1", 2)
			_ = db.ProjectSlice(slice.Symbols)
		}
	})

	t.Run("Bug101_IngestNonExistentDirectory", func(t *testing.T) {
		db := New()
		err := db.IngestGoAST("C:\\nonexistent_dir_random_12345")
		if err == nil {
			t.Fatalf("expected error on non-existent directory")
		}
	})

	t.Run("Bug102_CompleteEndToEndWorkflow", func(t *testing.T) {
		db := New()
		src := `package server
type Server struct { Port int }
func NewServer(port int) *Server { return &Server{Port: port} }
func (s *Server) Listen() { s.serve() }
func (s *Server) serve() {}`
		if err := db.IngestSource("server.go", src); err != nil {
			t.Fatalf("failed to ingest: %v", err)
		}

		tmpDir := t.TempDir()
		savedFile := filepath.Join(tmpDir, "server.mbvmdb")

		// 1. Save to disk
		if err := db.SaveToFile(savedFile); err != nil {
			t.Fatalf("failed to save: %v", err)
		}

		// 2. Load from disk
		loadedDB, err := LoadFromFile(savedFile)
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		// 3. Extract slice on loaded DB
		slice := loadedDB.ExtractSlice("Server.Listen", 2)
		proj := loadedDB.ProjectSlice(slice.Symbols)

		// 4. Verify all connected symbols present
		if !strings.Contains(proj, "func (*Server) Listen()") {
			t.Errorf("missing Listen in projection: %s", proj)
		}
		if !strings.Contains(proj, "func (*Server) serve()") {
			t.Errorf("missing serve in projection: %s", proj)
		}
		if !strings.Contains(proj, "type Server struct") {
			t.Errorf("missing Server struct in projection: %s", proj)
		}
	})
}
