package impact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// fingerprintGoEnv records the effective persistent Go environment file.
//
// os.Getenv alone misses settings saved by "go env -w": those are read by
// child Go processes but do not become process environment variables.
// GOENV=off explicitly disables this file. Otherwise Go defaults to
// UserConfigDir()/go/env (including %APPDATA% on Windows).
//
// A missing file is represented explicitly, so creating it invalidates a
// prior key. The helper never invokes the Go executable.
func fingerprintGoEnv() (string, error) {
	location := os.Getenv("GOENV")
	if location == "off" {
		return "off", nil
	}
	if location == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate persistent Go environment: %w", err)
		}
		location = filepath.Join(config, "go", "env")
	}
	path, err := filepath.Abs(location)
	if err != nil {
		return "", fmt.Errorf("resolve GOENV path: %w", err)
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return path + ":missing", nil
	}
	if err != nil {
		return "", fmt.Errorf("open GOENV file: %w", err)
	}
	// GOENV is a small configuration file. Its contents must be inspected
	// too: -modfile/-overlay and workspaces point outside the module root,
	// and hashing the config file alone cannot track those external inputs.
	data, readErr := io.ReadAll(io.LimitReader(file, 1<<20+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("read GOENV file: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close GOENV file: %w", closeErr)
	}
	if len(data) > 1<<20 {
		return "", fmt.Errorf("persistent Go environment exceeds 1 MiB")
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "GOFLAGS":
			if strings.Contains(value, "-modfile") || strings.Contains(value, "-overlay") {
				return "", fmt.Errorf("persistent GOFLAGS points to external modfile/overlay")
			}
		case "GOWORK":
			if value != "" && value != "off" {
				return "", fmt.Errorf("persistent GOWORK may reference an external workspace")
			}
		}
	}
	digest := sha256.Sum256(data)
	return path + ":" + hex.EncodeToString(digest[:]), nil
}
