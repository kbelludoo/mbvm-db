package impact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleCachePersistentGoEnvInvalidates(t *testing.T) {
	root := cacheFixture(t)
	configPath := filepath.Join(t.TempDir(), "go-env")
	t.Setenv("GOENV", configPath)
	t.Setenv("GOFLAGS", "")
	if err := os.WriteFile(configPath, []byte("GOFLAGS=-tags=cachephasea\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cache := NewModuleCache(1)
	_, cold, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || cold.CacheHit {
		t.Fatalf("initial load: stats=%+v err=%v", cold, err)
	}
	_, warm, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || !warm.CacheHit || warm.GoListCalls != 0 {
		t.Fatalf("warm load: stats=%+v err=%v", warm, err)
	}

	before, err := os.Stat(configPath)
	if err != nil { t.Fatal(err) }
	// Keep identical byte count AND modification timestamp. This simulates
	// an external go env -w edit that naive mtime-based caches miss.
	if err := os.WriteFile(configPath, []byte("GOFLAGS=-tags=cachephaseb\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(configPath, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	_, changed, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || changed.CacheHit || changed.GoListCalls != 1 {
		t.Fatalf("changed persistent Go config must reload: stats=%+v err=%v", changed, err)
	}
	_, reused, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || !reused.CacheHit {
		t.Fatalf("stable new GOENV should reuse graph: stats=%+v err=%v", reused, err)
	}
}

func TestModuleCacheGoEnvFileCreationAndOffMode(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing-env")
	t.Setenv("GOENV", configPath)
	missing, err := fingerprintGoEnv()
	if err != nil || !strings.HasSuffix(missing, ":missing") {
		t.Fatalf("missing env state: %q, %v", missing, err)
	}
	if err := os.WriteFile(configPath, []byte("GOFLAGS=-tags=created\n"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err := fingerprintGoEnv()
	if err != nil || created == missing {
		t.Fatalf("creating GOENV should change hash: %q -> %q, %v", missing, created, err)
	}
	// The off setting must never read a configured file.
	t.Setenv("GOENV", "off")
	off, err := fingerprintGoEnv()
	if err != nil || off != "off" {
		t.Fatalf("GOENV=off: %q, %v", off, err)
	}
}

func TestModuleCacheBypassesWhitespaceReplaceDirective(t *testing.T) {
	root := cacheFixture(t)
	mod := filepath.Join(root, "go.mod")
	for _, directive := range []string{
		"replace\tmodule.example/foo => ../foo\n",
		"replace\t(\n module.example/foo => ../foo\n)\n",
	} {
		contents := "module example.test/multipkg\n\ngo 1.23\n" + directive
		if err := os.WriteFile(mod, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		fp, err := fingerprintModule(root)
		if err != nil || fp.cacheable || !strings.Contains(fp.reason, "replace") {
			t.Fatalf("whitespace replace must bypass: %+v, %v", fp, err)
		}
	}
}

func TestModuleCachePersistentExternalBuildInputsBypass(t *testing.T) {
	root := cacheFixture(t)
	configPath := filepath.Join(t.TempDir(), "go-env")
	t.Setenv("GOENV", configPath)
	for _, content := range []string{
		"GOFLAGS=-modfile=/tmp/independent.mod\n",
		"GOFLAGS=-overlay=/tmp/independent.json\n",
		"GOWORK=/tmp/independent.work\n",
	} {
		if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		fp, err := fingerprintModule(root)
		if err != nil || fp.cacheable || !strings.Contains(fp.reason, "persistent Go environment") {
			t.Fatalf("external build input must bypass cache: config=%q fp=%+v err=%v", content, fp, err)
		}
	}
}
