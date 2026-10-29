package competitorvalidation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const fixtureSource = `package fixture
func Target() {}
func Direct() { Target() }
type Runner struct{}
func (*Runner) Method() { Target() }
func Closure() { f := func(){ Target() }; f() }
func Async() { go Target(); defer Target() }
func Values() { f := Target; f() }
func Generic[T ~int](x T) {}
func GenericCaller() { Generic[int](1) }
func Unused() {}
func Recursive() { Recursive() }
func Switch() { Target() }
`

func makeFixture(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "child"), 0700); err != nil {
		return fmt.Errorf("create fixture: %w", err)
	}
	files := map[string]string{
		"go.mod":         "module example.com/callfixture\n\ngo 1.27\n",
		"lib.go":         fixtureSource,
		"child/child.go": "package child\nimport f \"example.com/callfixture\"\nfunc Cross(){ f.Target() }\n",
		"lib_test.go":    "package fixture\nimport \"testing\"\nfunc TestOnly(t *testing.T){ Target() }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			return fmt.Errorf("write fixture: %w", err)
		}
	}
	return nil
}

func fixtureTruth() map[string][]string {
	p := "example.com/callfixture."
	m := map[string][]string{}
	for _, name := range []string{"Target", "Direct", "Closure", "Async", "Values", "Generic", "GenericCaller", "Unused", "Recursive", "Switch"} {
		m[p+name] = []string{}
	}
	m["example.com/callfixture/child.Cross"] = []string{}
	m[p+"Target"] = []string{p + "(*Runner).Method", p + "Async", p + "Closure", p + "Direct", p + "Switch", "example.com/callfixture/child.Cross"}
	m[p+"Generic"] = []string{p + "GenericCaller"}
	for _, xs := range m {
		slices.Sort(xs)
	}
	return m
}

func (c *lspClient) fixtureOpen(root string) error {
	// Use a concrete type to keep transport fields explicit.
	var params struct {
		TextDocument struct {
			URI        string `json:"uri"`
			LanguageID string `json:"languageId"`
			Version    int    `json:"version"`
			Text       string `json:"text"`
		} `json:"textDocument"`
	}
	params.TextDocument.URI = fileURI(filepath.Join(root, "lib.go"))
	params.TextDocument.LanguageID = "go"
	params.TextDocument.Version = 1
	params.TextDocument.Text = fixtureSource
	b, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode fixture open: %w", err)
	}
	return c.notify("textDocument/didOpen", b)
}

func editFixture(root string, c *lspClient) error {
	text := strings.Replace(fixtureSource, "func Switch() { Target() }", "func Switch() { Unused() }", 1)
	if err := os.WriteFile(filepath.Join(root, "lib.go"), []byte(text), 0600); err != nil {
		return fmt.Errorf("save changed fixture: %w", err)
	}
	params := struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}{}
	params.TextDocument.URI = fileURI(filepath.Join(root, "lib.go"))
	params.TextDocument.Version = 2
	params.ContentChanges = append(params.ContentChanges, struct {
		Text string `json:"text"`
	}{text})
	b, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode fixture change: %w", err)
	}
	if err := c.notify("textDocument/didChange", b); err != nil {
		return err
	}
	save, _ := json.Marshal(struct {
		TextDocument document `json:"textDocument"`
	}{document{params.TextDocument.URI}})
	return c.notify("textDocument/didSave", save)
}
