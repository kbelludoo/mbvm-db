package mbvmdb

import (
	"fmt"
	"strings"
	"testing"
)

func TestSlice_DeterministicProjection(t *testing.T) {
	db := New()
	src := `package alpha

type User struct {
	ID int
	Name string
}

type Order struct {
	Total float64
}

func CreateUser() *User { return nil }
func ProcessOrder() {}
`
	err := db.IngestSource("alpha.go", src)
	if err != nil {
		t.Fatalf("IngestSource failed: %v", err)
	}

	slice := db.ExtractSlice("CreateUser", 2)
	proj1 := db.ProjectSlice(slice.Symbols)
	proj2 := db.ProjectSlice(slice.Symbols)

	if proj1 != proj2 {
		t.Fatalf("projection must be 100%% deterministic across runs:\n1:\n%s\n2:\n%s", proj1, proj2)
	}
}

func TestSlice_CircularDependencyLoop(t *testing.T) {
	db := New()
	src := `package cycle

func A() { B() }
func B() { C() }
func C() { A() }
`
	err := db.IngestSource("cycle.go", src)
	if err != nil {
		t.Fatalf("IngestSource failed: %v", err)
	}

	// Should not enter infinite loop and should find all 3 functions
	slice := db.ExtractSlice("A", 10)
	if len(slice.Symbols) != 3 {
		t.Fatalf("expected 3 symbols in cycle, got %d", len(slice.Symbols))
	}
}

func TestSlice_DepthLimiting(t *testing.T) {
	db := New()
	src := `package chain

func Step1() { Step2() }
func Step2() { Step3() }
func Step3() { Step4() }
func Step4() {}
`
	err := db.IngestSource("chain.go", src)
	if err != nil {
		t.Fatalf("IngestSource failed: %v", err)
	}

	// Depth 0: only Step1
	slice0 := db.ExtractSlice("Step1", 0)
	if len(slice0.Symbols) != 1 {
		t.Fatalf("depth 0 expected 1 symbol, got %d", len(slice0.Symbols))
	}

	// Depth 1: Step1 + Step2
	slice1 := db.ExtractSlice("Step1", 1)
	if len(slice1.Symbols) != 2 {
		t.Fatalf("depth 1 expected 2 symbols, got %d", len(slice1.Symbols))
	}

	// Depth 2: Step1 + Step2 + Step3
	slice2 := db.ExtractSlice("Step1", 2)
	if len(slice2.Symbols) != 3 {
		t.Fatalf("depth 2 expected 3 symbols, got %d", len(slice2.Symbols))
	}
}

func TestSlice_TypeFieldResolution(t *testing.T) {
	db := New()
	src := `package domain

type Address struct {
	City string
}

type Customer struct {
	Addr *Address
	Tags []string
}

func GetCustomer() *Customer {
	return nil
}
`
	err := db.IngestSource("domain.go", src)
	if err != nil {
		t.Fatalf("IngestSource failed: %v", err)
	}

	// Slicing Customer with depth 1 should pull Address struct
	slice := db.ExtractSlice("Customer", 1)
	proj := db.ProjectSlice(slice.Symbols)

	if !strings.Contains(proj, "type Address struct") {
		t.Fatalf("expected Address to be resolved from Customer.Addr field, got:\n%s", proj)
	}
}

func TestSlice_TokenBudgetLimiting(t *testing.T) {
	db := New()
	var sb strings.Builder
	sb.WriteString("package budget\n")
	sb.WriteString("func Core() { S1(); S2(); S3(); S4(); S5() }\n")
	for i := 1; i <= 5; i++ {
		sb.WriteString(fmt.Sprintf("func S%d() {}\n", i))
	}
	_ = db.IngestSource("budget.go", sb.String())

	// Full slice has 6 symbols
	full := db.ExtractSlice("Core", 1)
	if len(full.Symbols) != 6 {
		t.Fatalf("expected 6 symbols in full slice, got %d", len(full.Symbols))
	}

	// Budget slice constrained to 30 tokens should only include Core and top 1-2 symbols
	budgetSlice := db.ExtractBudgetSlice("Core", 1, 35)
	if budgetSlice.TokensEstimate > 35 {
		t.Fatalf("tokens %d exceeded budget of 35", budgetSlice.TokensEstimate)
	}
	if len(budgetSlice.Symbols) == 0 || budgetSlice.Symbols[0].Name != "Core" {
		t.Fatalf("target symbol Core must be prioritized in budget slice")
	}
}

func TestSlice_BitsetOperations(t *testing.T) {
	bs := NewBitset(100)
	if bs.Has(10) {
		t.Fatalf("bit 10 should not be set")
	}
	bs.Set(10)
	if !bs.Has(10) {
		t.Fatalf("bit 10 should be set")
	}
	// Test expansion past initial capacity
	bs.Set(150)
	if !bs.Has(150) {
		t.Fatalf("expanded bit 150 should be set")
	}
}

