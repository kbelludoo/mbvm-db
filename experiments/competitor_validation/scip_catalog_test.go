package competitorvalidation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSCIPCataloguePreservesExistingRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "fixture")
	out := filepath.Join(parent, "catalogue.json")
	if err := ExportSCIPCatalogue(root, out, true); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "lib.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ExportSCIPCatalogue(root, out, true); err == nil {
		t.Fatal("existing fixture root was accepted for overwrite")
	}
	after, err := os.ReadFile(filepath.Join(root, "lib.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("fixture source changed")
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var c SCIPCatalogue
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Targets) != 11 || c.SourceSHA256 == "" || len(c.Calls) == 0 {
		t.Fatalf("incomplete catalogue: %d targets %d call positions", len(c.Targets), len(c.Calls))
	}
}
