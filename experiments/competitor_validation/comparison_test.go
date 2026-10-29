package competitorvalidation

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

func TestIndependentFixtureTruth(t *testing.T) {
	root := t.TempDir()
	if err := makeFixture(root); err != nil {
		t.Fatal(err)
	}
	g, err := impact.LoadModule(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := makeCatalogue(g)
	if err != nil {
		t.Fatal(err)
	}
	want := fixtureTruth()
	if len(c.Targets) != len(want) {
		t.Fatalf("targets=%d, labelled=%d", len(c.Targets), len(want))
	}
	for name, expected := range want {
		got, ok := c.Targets[name]
		if !ok || !slices.Equal(got.Want, expected) {
			t.Errorf("%s callers=%v want=%v", name, got.Want, expected)
		}
	}
}

func TestNormalizeValueReferencesTestsAndSelf(t *testing.T) {
	root := t.TempDir()
	if err := makeFixture(root); err != nil {
		t.Fatal(err)
	}
	g, err := impact.LoadModule(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := makeCatalogue(g)
	if err != nil {
		t.Fatal(err)
	}
	uri := fileURI(filepath.Join(root, "lib.go"))
	// Literal LSP source positions, independent of c.Calls and MBVM edges.
	calls := []incomingCall{
		{From: hierarchyItem{URI: uri}, FromRanges: []span{{Start: position{2, 16}}, {Start: position{7, 21}}}},
		{From: hierarchyItem{URI: fileURI(filepath.Join(root, "lib_test.go"))}, FromRanges: []span{{Start: position{2, 32}}}},
	}
	got, dropped, err := c.normalize(calls, "example.com/callfixture.Target")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"example.com/callfixture.Direct"}) || dropped != 2 {
		t.Fatalf("got=%v dropped=%d", got, dropped)
	}
	self := []incomingCall{{From: hierarchyItem{URI: uri}, FromRanges: []span{{Start: position{11, 19}}}}}
	got, dropped, err = c.normalize(self, "example.com/callfixture.Recursive")
	if err != nil || len(got) != 0 || dropped != 1 {
		t.Fatalf("self=%v dropped=%d err=%v", got, dropped, err)
	}
}

func TestWireFramingUTF16AndURI(t *testing.T) {
	for _, body := range []string{`{"id":1,"result":[]}`, `{"text":"ação 🧠"}`} {
		frame := fmt.Sprintf("Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
		b, err := readFrame(bufio.NewReader(strings.NewReader(frame)))
		if err != nil || string(b) != body {
			t.Fatalf("frame=%q err=%v", b, err)
		}
	}
	for _, bad := range []string{"Content-Length: 8\r\n\r\n{}", "Content-Length: -1\r\n\r\n", "Content-Length: 33554433\r\n\r\n"} {
		if _, err := readFrame(bufio.NewReader(strings.NewReader(bad))); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	text := []byte("ação\n🧠 Target")
	p := point(text, len([]byte("ação\n🧠 ")))
	if p != (position{1, 3}) {
		t.Fatalf("UTF-16 position=%v", p)
	}
	path := filepath.Join(t.TempDir(), "ação space.go")
	got, err := uriPath(fileURI(path))
	if err != nil || got != path {
		t.Fatalf("URI round trip %q %v", got, err)
	}
}

func TestSnapshotDetectsSourceAndModuleChanges(t *testing.T) {
	root := t.TempDir()
	if err := makeFixture(root); err != nil {
		t.Fatal(err)
	}
	before, err := snapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "lib.go")
	if err := os.WriteFile(path, []byte(strings.Replace(fixtureSource, "Target() {}", "Target() { println(1) }", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := snapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("missed source change")
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/other\n\ngo 1.27\n"), 0600); err != nil {
		t.Fatal(err)
	}
	last, err := snapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if after == last {
		t.Fatal("missed module configuration change")
	}
}
