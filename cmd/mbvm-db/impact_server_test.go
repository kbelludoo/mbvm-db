package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func serverFixture(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	root := t.TempDir()
	for name, body := range map[string]string{"go.mod": "module example.test/server\n\ngo 1.27\n", "server.go": "package server\nfunc A(){}\nfunc B(){A()}\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, "example.test/server.A"
}

func decodeServerResponses(t *testing.T, raw []byte) []impactResponse {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	out := []impactResponse{}
	for {
		var response impactResponse
		err := decoder.Decode(&response)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, response)
	}
	return out
}

func TestImpactServerCacheReuseInvalidationAndBadRequestIsolation(t *testing.T) {
	root, symbol := serverFixture(t)
	input := strings.Join([]string{
		`{"id":"cold","symbol":"` + symbol + `"}`,
		`{"id":"warm","symbol":"` + symbol + `"}`,
		`{"id":"invalidate","op":"invalidate"}`,
		`{"id":"reload","symbol":"` + symbol + `"}`,
		`{"id":"bad","op":"unsupported"}`,
		`{"id":"extra","symbol":"` + symbol + `","root":"outside"}`,
		`{"id":"tail","symbol":"` + symbol + `"} {}`,
		`{"id":"still-valid","symbol":"` + symbol + `"}`,
	}, "\n")
	var output bytes.Buffer
	if err := runImpactServer([]string{"-root", root}, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	rows := decodeServerResponses(t, output.Bytes())
	if len(rows) != 8 {
		t.Fatalf("got %d responses", len(rows))
	}
	if rows[0].Stats.CacheHit || !rows[1].Stats.CacheHit || rows[1].Stats.GoListCalls != 0 || !rows[2].Invalidated || rows[3].Stats.CacheHit {
		t.Fatal("wrong cache/invalidation behavior")
	}
	for _, i := range []int{4, 5, 6} {
		if rows[i].Error == "" || rows[i].Impact != nil {
			t.Fatal("invalid request was executed")
		}
	}
	if rows[7].Error != "" || !rows[7].Stats.CacheHit || !slices.Equal(rows[7].Impact.DirectCallers, []string{"example.test/server.B"}) {
		t.Fatal("bad request poisoned later query")
	}
}

type editBetweenRequests struct {
	first  *strings.Reader
	second *strings.Reader
	edit   func() error
	edited bool
}

func (r *editBetweenRequests) Read(p []byte) (int, error) {
	if r.first.Len() > 0 {
		return r.first.Read(p)
	}
	if !r.edited {
		if err := r.edit(); err != nil {
			return 0, err
		}
		r.edited = true
	}
	return r.second.Read(p)
}

func TestImpactServerRechecksSourceBetweenRequests(t *testing.T) {
	root, symbol := serverFixture(t)
	request := `{"symbol":"` + symbol + `"}` + "\n"
	input := &editBetweenRequests{first: strings.NewReader(request), second: strings.NewReader(request), edit: func() error {
		return os.WriteFile(filepath.Join(root, "server.go"), []byte("package server\nfunc A(){}\nfunc B(){A()}\nfunc C(){A()}\n"), 0600)
	}}
	var output bytes.Buffer
	if err := runImpactServer([]string{"-root", root}, input, &output); err != nil {
		t.Fatal(err)
	}
	rows := decodeServerResponses(t, output.Bytes())
	if len(rows) != 2 {
		t.Fatal("missing responses")
	}
	if rows[1].Stats.CacheHit || rows[1].Stats.GoListCalls != 1 || !slices.Equal(rows[1].Impact.DirectCallers, []string{"example.test/server.B", "example.test/server.C"}) {
		t.Fatal("source change returned stale callers")
	}
}

func TestImpactServerLimitsInputAndRejectsMissingModule(t *testing.T) {
	root, _ := serverFixture(t)
	if err := runImpactServer([]string{"-root", root}, strings.NewReader(strings.Repeat("x", (1<<20)+1)), io.Discard); err == nil {
		t.Fatal("oversized input accepted")
	}
	if err := runImpactServer([]string{"-root", t.TempDir()}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("missing module accepted")
	}
	if err := runImpactServer([]string{"unexpected"}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("unexpected positional argument accepted")
	}
}
