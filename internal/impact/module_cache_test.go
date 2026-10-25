package impact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const cacheTestSymbol = "example.test/multipkg/api.(A).Apply"

func cacheFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOFLAGS", "")
	return moduleFixture(t)
}

func TestModuleCacheHitAndSourceInvalidation(t *testing.T) {
	root := cacheFixture(t)
	cache := NewModuleCache(2)
	first, stats, err := cache.Impact(root, cacheTestSymbol)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CacheHit || stats.GoListCalls != 1 || stats.Packages != 2 {
		t.Fatalf("first lookup should build graph: %+v", stats)
	}
	originalDirect := append([]string(nil), first.DirectCallers...)
	first.DirectCallers[0] = "corrupt-the-report-not-the-cache"
	second, hot, err := cache.Impact(root, cacheTestSymbol)
	if err != nil {
		t.Fatal(err)
	}
	if !hot.CacheHit || hot.GoListCalls != 0 || hot.Symbols != stats.Symbols ||
		hot.Edges != stats.Edges || !equalStrings(second.DirectCallers, originalDirect) {
		t.Fatalf("cached report was stale or mutated: first=%+v hot=%+v second=%+v", stats, hot, second)
	}
	api := filepath.Join(root, "api", "api.go")
	file, err := os.OpenFile(api, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("\nfunc NewCacheCall(a A)int{return a.Apply(3)}\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("fixture update: %v %v", writeErr, closeErr)
	}
	_, refreshed, err := cache.Impact(root, cacheTestSymbol)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.CacheHit || refreshed.GoListCalls != 1 ||
		refreshed.Symbols != stats.Symbols+1 || refreshed.Edges != stats.Edges+1 {
		t.Fatalf("source edit returned cached stale graph: first=%+v refreshed=%+v", stats, refreshed)
	}
	_, again, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || !again.CacheHit {
		t.Fatalf("post-update reuse: %+v %v", again, err)
	}
}

func TestModuleCacheGoModAndBuildContextInvalidate(t *testing.T) {
	root := cacheFixture(t)
	cache := NewModuleCache(1)
	_, cold, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || cold.CacheHit {
		t.Fatalf("initial load %+v %v", cold, err)
	}
	mod := filepath.Join(root, "go.mod")
	file, err := os.OpenFile(mod, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("\n// fingerprint must observe metadata edits\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	_, afterMod, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || afterMod.CacheHit {
		t.Fatalf("go.mod did not invalidate %+v %v", afterMod, err)
	}
	t.Setenv("GOFLAGS", "-tags=phase3cachefixture")
	_, afterFlags, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || afterFlags.CacheHit {
		t.Fatalf("build tags did not invalidate %+v %v", afterFlags, err)
	}
	_, hit, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || !hit.CacheHit {
		t.Fatalf("same build context did not reuse graph %+v %v", hit, err)
	}
	if err := cache.Invalidate(root); err != nil {
		t.Fatal(err)
	}
	_, manual, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || manual.CacheHit {
		t.Fatalf("Invalidate ineffective %+v %v", manual, err)
	}
	cache.Clear()
	_, cleared, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || cleared.CacheHit {
		t.Fatalf("Clear ineffective %+v %v", cleared, err)
	}
}

func TestModuleCacheBypassesUnknownExternalInputs(t *testing.T) {
	root := cacheFixture(t)
	mod := filepath.Join(root, "go.mod")
	if err := os.WriteFile(mod, []byte("module example.test/multipkg\n\ngo 1.23\nreplace example.test/unused => ../outside\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fp, err := fingerprintModule(root)
	if err != nil || fp.cacheable || !strings.Contains(fp.reason, "replace") {
		t.Fatalf("replace must bypass cache: %+v %v", fp, err)
	}
	t.Setenv("GOWORK", "off")
	if err := os.WriteFile(mod, []byte("module example.test/multipkg\n\ngo 1.23\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0700); err != nil {
		t.Fatal(err)
	}
	fp, err = fingerprintModule(root)
	if err != nil || fp.cacheable || !strings.Contains(fp.reason, "vendor") {
		t.Fatalf("vendor must bypass cache: %+v %v", fp, err)
	}
}

func TestModuleCacheConcurrentQueriesAndEviction(t *testing.T) {
	root := cacheFixture(t)
	cache := NewModuleCache(1)
	const workers = 8
	var wg sync.WaitGroup
	failures := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := cache.Impact(root, cacheTestSymbol)
			if err != nil {
				failures <- err
				return
			}
			if len(result.DirectCallers) != 1 {
				failures <- fmt.Errorf("incorrect concurrent reachability: %+v", result)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	_, after, err := cache.Impact(root, cacheTestSymbol)
	if err != nil || !after.CacheHit {
		t.Fatalf("concurrent cache not reusable %+v %v", after, err)
	}
}

// This benchmark measures the per-process warm-query fast path,
// including full SHA-256 fingerprint verification of source contents.
func BenchmarkModuleCacheWarmImpact(b *testing.B) {
	root := b.TempDir()
	for _, p := range []string{"api", "client"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0755); err != nil {
			b.Fatal(err)
		}
	}
	contents := map[string]string{
		"go.mod":           "module example.test/cachebench\n\ngo 1.23\n",
		"api/api.go":       "package api\nfunc Run(x int)int{return x}\n",
		"client/client.go": "package client\nimport \"example.test/cachebench/api\"\nfunc Use()int{return api.Run(2)}\n",
	}
	for name, body := range contents {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0600); err != nil {
			b.Fatal(err)
		}
	}
	b.Setenv("GOWORK", "off")
	b.Setenv("GOPROXY", "off")
	b.Setenv("GOSUMDB", "off")
	b.Setenv("GOFLAGS", "")
	cache := NewModuleCache(1)
	target := "example.test/cachebench/api.Run"
	if _, _, err := cache.Impact(root, target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var last ModuleCacheQueryStats
	var fingerprints, traversals float64
	for n := 0; n < b.N; n++ {
		_, stats, err := cache.Impact(root, target)
		if err != nil {
			b.Fatal(err)
		}
		if !stats.CacheHit {
			b.Fatal("unexpected cold result")
		}
		last = stats
	}
	b.StopTimer()
	b.ReportMetric(float64(last.GoListCalls), "go-list/op")
	b.ReportMetric(float64(last.Edges), "edges/op")
}

func TestModuleCacheFingerprintGoEnvAndEmbedProtection(t *testing.T) {
	root := cacheFixture(t)
	config := filepath.Join(t.TempDir(), "go-env")
	t.Setenv("GOENV", config)
	if err := os.WriteFile(config, []byte("GOFLAGS=-tags=first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := fingerprintModule(root)
	if err != nil || !first.cacheable {
		t.Fatalf("initial config: %+v %v", first, err)
	}
	if err := os.WriteFile(config, []byte("GOFLAGS=-tags=other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := fingerprintModule(root)
	if err != nil || !second.cacheable || first.hash == second.hash {
		t.Fatalf("go env -w settings changed without cache invalidation %+v %+v %v", first, second, err)
	}
	t.Setenv("GOENV", "off")
	t.Setenv("GOWORK", "")
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.23\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workspace, err := fingerprintModule(root)
	if err != nil || workspace.cacheable || !strings.Contains(workspace.reason, "workspace") {
		t.Fatalf("workspace inputs must bypass: %+v %v", workspace, err)
	}
	t.Setenv("GOWORK", "off")
	if err := os.Remove(filepath.Join(root, "go.work")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "api", "embedded.go")
	if err := os.WriteFile(file, []byte("package api\nimport _ \"embed\"\n//go:embed asset.txt\nvar asset string\n"), 0600); err != nil {
		t.Fatal(err)
	}
	embedded, err := fingerprintModule(root)
	if err != nil || embedded.cacheable || !strings.Contains(embedded.reason, "embed") {
		t.Fatalf("embed assets not protected from stale cache: %+v %v", embedded, err)
	}
}

func BenchmarkModuleCacheWarmImpactReal(b *testing.B) {
	if os.Getenv("MBVM_BENCH_REAL_MODULE") != "1" {
		b.Skip("set MBVM_BENCH_REAL_MODULE=1 to benchmark the real module cache")
	}
	cache := NewModuleCache(1)
	const root = "../.."
	const target = "github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New"
	if _, _, err := cache.Impact(root, target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var last ModuleCacheQueryStats
	for n := 0; n < b.N; n++ {
		_, stats, err := cache.Impact(root, target)
		if err != nil {
			b.Fatal(err)
		}
		if !stats.CacheHit {
			b.Fatal("full-module warm cache unexpectedly missed")
		}
		last = stats
		fingerprints += stats.FingerprintMilliseconds
		traversals += stats.ImpactMilliseconds
	}
	b.StopTimer()
	b.ReportMetric(float64(last.GoListCalls), "go-list/op")
	b.ReportMetric(float64(last.Symbols), "symbols/op")
	b.ReportMetric(float64(last.Edges), "edges/op")
	b.ReportMetric(fingerprints/float64(b.N), "fingerprint-ms/op")
	b.ReportMetric(traversals/float64(b.N), "impact-ms/op")
}

func TestModuleCacheStreamingFingerprintLongLinesCRLF(t *testing.T) {
 root:=cacheFixture(t)
 source:=filepath.Join(root,"api","large.go")
 // Test beyond the 32 KiB streaming buffer without materializing a
 // package-size byte slice on every warm cache lookup.
 contents:="package api\r\n// "+strings.Repeat("x",96<<10)+"\r\nfunc Streamed() int { return 1 }\r\n"
 if err:=os.WriteFile(source,[]byte(contents),0600);err!=nil{t.Fatal(err)}
 initial,err:=fingerprintModule(root)
 if err!=nil||!initial.cacheable{t.Fatalf("initial fingerprint: %+v %v",initial,err)}
 again,err:=fingerprintModule(root)
 if err!=nil||again.hash!=initial.hash{t.Fatalf("identical source bytes changed hash: %+v %+v %v",initial,again,err)}
 stat,err:=os.Stat(source);if err!=nil{t.Fatal(err)}
 changed:=strings.Replace(contents,"return 1","return 2",1)
 if len(changed)!=len(contents){t.Fatal("fixture must keep original size")}
 if err:=os.WriteFile(source,[]byte(changed),0600);err!=nil{t.Fatal(err)}
 if err:=os.Chtimes(source,stat.ModTime(),stat.ModTime());err!=nil{t.Fatal(err)}
 updated,err:=fingerprintModule(root)
 if err!=nil||!updated.cacheable||updated.hash==initial.hash{
  t.Fatalf("same-size, same-mtime source edit escaped SHA-256: before=%+v after=%+v err=%v",initial,updated,err)
 }
 // An embed directive after a long scanner line must still force a
 // conservative bypass, including Windows CRLF line endings.
 changed+="//go:embed some-assets/*\r\n"
 if err:=os.WriteFile(source,[]byte(changed),0600);err!=nil{t.Fatal(err)}
 disabled,err:=fingerprintModule(root)
 if err!=nil||disabled.cacheable||!strings.Contains(disabled.reason,"embed"){
  t.Fatalf("long-line embed detection lost: %+v %v",disabled,err)
 }
}

func TestModuleCacheMeasuresActualImpactTraversal(t *testing.T) {
 root:=cacheFixture(t)
 cache:=NewModuleCache(1)
 if _,_,err:=cache.Impact(root,cacheTestSymbol);err!=nil{t.Fatal(err)}
 _,stats,err:=cache.Impact(root,cacheTestSymbol)
 if err!=nil{t.Fatal(err)}
 if !stats.CacheHit||stats.GoListCalls!=0||stats.ImpactMilliseconds<0||
  stats.FingerprintMilliseconds<0||stats.TotalMilliseconds<=0||
  stats.ImpactMilliseconds>stats.TotalMilliseconds||
  stats.FingerprintMilliseconds>stats.TotalMilliseconds{
  t.Fatalf("independent impact/fingerprint timings invalid: %+v",stats)
 }
 t.Logf("warm cache: fingerprint=%.3f ms, impact-only=%.3f ms, total=%.3f ms",
  stats.FingerprintMilliseconds,stats.ImpactMilliseconds,stats.TotalMilliseconds)
}

func BenchmarkModuleFingerprintOnlyReal(b *testing.B) {
 if os.Getenv("MBVM_BENCH_REAL_MODULE") != "1" {
  b.Skip("set MBVM_BENCH_REAL_MODULE=1 to measure the real Go source fingerprint")
 }
 root:="../.."
 first,err:=fingerprintModule(root)
 if err!=nil{b.Fatal(err)}
 if !first.cacheable{b.Fatalf("module cache disabled: %s",first.reason)}
 b.ReportAllocs()
 b.ResetTimer()
 for n:=0;n<b.N;n++ {
  fp,err:=fingerprintModule(root)
  if err!=nil{b.Fatal(err)}
  if !fp.cacheable||fp.hash!=first.hash{
   b.Fatalf("unexpected source changes during fingerprint benchmark: %+v",fp)
  }
 }
}
