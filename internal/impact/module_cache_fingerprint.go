package impact

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type moduleFingerprint struct {
	hash      string
	cacheable bool
	reason    string
}

// Only inputs understood by the source/typechecking importer are cached.
// In uncertain configurations, disable reuse rather than return stale data.
// Fingerprinting does not invoke the Go subprocess.
func fingerprintModule(root string) (moduleFingerprint, error) {
	result := moduleFingerprint{cacheable: true}
	disable := func(reason string) (moduleFingerprint, error) {
		result.cacheable = false
		result.reason = reason
		return result, nil
	}
	// A local replacement can change without any file in the main module
	// changing, so the first cache version deliberately does not support it.
	mod, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return result, err
	}
	scanner := bufio.NewScanner(mod)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "//", 2)[0])
		if fields := strings.Fields(line); len(fields) > 0 &&
			(fields[0] == "replace" || strings.HasPrefix(fields[0], "replace(")) {
			mod.Close()
			return disable("local or remote replace directives require a fresh module load")
		}
	}
	scanErr := scanner.Err()
	closeErr := mod.Close()
	if scanErr != nil {
		return result, scanErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	if os.Getenv("GOWORK") != "" && os.Getenv("GOWORK") != "off" {
		return disable("explicit Go workspace is not cached")
	}
	if os.Getenv("GOWORK") != "off" {
		for parent := root; ; parent = filepath.Dir(parent) {
			_, err := os.Stat(filepath.Join(parent, "go.work"))
			if err == nil {
				return disable("Go workspace in parent directory is not cached")
			}
			if !os.IsNotExist(err) {
				return result, err
			}
			if filepath.Dir(parent) == parent {
				break
			}
		}
	}
	flags := os.Getenv("GOFLAGS")
	if strings.Contains(flags, "-modfile") || strings.Contains(flags, "-overlay") {
		return disable("external -modfile/-overlay is not cacheable")
	}
	if info, err := os.Stat(filepath.Join(root, "vendor")); err == nil && info.IsDir() {
		return disable("vendor tree may change compiler exports independently")
	} else if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	// Compiler changes in place must invalidate a graph without starting
	// go version on every warm hit. Toolchains installed by GOTOOLCHAIN are
	// differentiated by the environment + module Go/toolchain directives.
	goPath, err := exec.LookPath("go")
	if err != nil {
		return result, err
	}
	goStat, err := os.Stat(goPath)
	if err != nil {
		return result, err
	}
	digest := sha256.New()
	write := func(a, b string) {
		io.WriteString(digest, a)
		digest.Write([]byte{0})
		io.WriteString(digest, b)
		digest.Write([]byte{0})
	}
	write("cache-schema", "mbvm-module-cache-v1")
	write("runtime.Version", runtime.Version())
	write("runtime.GOOS", runtime.GOOS)
	write("runtime.GOARCH", runtime.GOARCH)
	write("go-executable", goPath)
	write("go-executable-stat", fmt.Sprintf("%d/%d", goStat.Size(), goStat.ModTime().UnixNano()))
	// Fingerprint persistent toolchain defaults, including GOFLAGS which
	// go env -w stores outside the process environment. Reject external
	// overlays/modfiles and workspaces that the module digest cannot see.
	goEnvState, err := fingerprintGoEnv()
	if err != nil {
		return disable("cannot fingerprint persistent Go environment: " + err.Error())
	}
	write("go-env-file", goEnvState)
	for _, key := range []string{
		"GOOS", "GOARCH", "GOAMD64", "GOARM", "GOARM64", "GO386", "GOEXPERIMENT",
		"GOFLAGS", "CGO_ENABLED", "CC", "CXX", "GOWORK", "GOTOOLCHAIN",
		"GO111MODULE", "GOPROXY", "GOSUMDB", "GONOSUMDB", "GONOPROXY",
		"GOPRIVATE", "GOMODCACHE", "GOPATH", "GOROOT", "GOENV",
	} {
		write("env:"+key, os.Getenv(key))
	}
	// Fingerprint content, not timestamps, so same-length edits with restored
	// modification times also invalidate the cached graph.
	filenames := []string{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			// A directory symlink could introduce source files outside the root.
			return fmt.Errorf("symlinks disallow cached source fingerprint: %s", path)
		}
		if d.IsDir() {
			if d.Name() == ".git" && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".go") ||
			name == "go.mod" || name == "go.sum" ||
			name == "go.work" || name == "go.work.sum" {
			filenames = append(filenames, path)
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "symlinks disallow cached source fingerprint") {
			return disable("symlink in module tree")
		}
		return result, err
	}
	sort.Strings(filenames)
	for _, path := range filenames {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return result, err
		}
		io.WriteString(digest, filepath.ToSlash(rel))
		digest.Write([]byte{0})
		f, err := os.Open(path)
		if err != nil {
			return result, err
		}
		// Compiler embed directives depend on non-Go files that are not
		// enumerated by this source-only hash. In that case, always reload.
		// Read as bytes while hashing, without evaluating arbitrary patterns.
		contents, readErr := io.ReadAll(f)
		closeErr := f.Close()
		if readErr != nil {
			return result, readErr
		}
		if closeErr != nil {
			return result, closeErr
		}
		if strings.HasSuffix(path, ".go") && containsGoEmbedDirective(contents) {
			return disable("go:embed assets are not tracked by this Go-source-only cache")
		}
		digest.Write(contents)
		digest.Write([]byte{0})
	}
	result.hash = hex.EncodeToString(digest.Sum(nil))
	return result, nil
}

func containsGoEmbedDirective(contents []byte) bool {
	s := bufio.NewScanner(bytes.NewReader(contents))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "//go:embed ") || strings.HasPrefix(line, "/*go:embed ") {
			return true
		}
	}
	return false
}
