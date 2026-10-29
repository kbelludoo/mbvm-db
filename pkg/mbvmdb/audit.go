package mbvmdb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AuditReport details the coverage of game mechanics and protocol by the bot codebase.
type AuditReport struct {
	TotalGameFeatures int
	CoveredFeatures   int
	CoveragePercent   float64
	CoveredList       []string
	MissingList       []string
}

// AuditBotCoverage scans a bot codebase directory against its game.mbvmdb database
// to identify all implemented and missing game mechanics, opcodes, tags, and hunts.
func AuditBotCoverage(botDir string) (*AuditReport, error) {
	gameDBPath := filepath.Join(botDir, "game.mbvmdb")
	if _, err := os.Stat(gameDBPath); err != nil {
		// Try game_protocol.mbvmdb
		gameDBPath = filepath.Join(botDir, "game_protocol.mbvmdb")
		if _, err := os.Stat(gameDBPath); err != nil {
			return nil, fmt.Errorf("arquivo game.mbvmdb não encontrado em %s: %w", botDir, err)
		}
	}

	gameDB, err := LoadFromFile(gameDBPath)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar %s: %w", gameDBPath, err)
	}

	// Read all Go source files in the bot directory to build a full text search index
	var allCode strings.Builder
	_ = filepath.WalkDir(botDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") {
			content, err := os.ReadFile(path)
			if err == nil {
				allCode.Write(content)
				allCode.WriteString("\n")
			}
		}
		return nil
	})
	codeStr := allCode.String()
	codeStrLower := strings.ToLower(codeStr)

	var covered []string
	var missing []string

	// For each game symbol, check if it is actively handled in code
	seen := make(map[string]bool)

	for _, sym := range gameDB.symbols {
		var checkTokens []string
		displayName := sym.Name

		if sym.Kind == "proto_message" {
			// Check if message fields/tags are handled
			for _, f := range sym.Fields {
				// e.g. "Tag 80: optional Enchant enchant"
				parts := strings.Fields(f)
				if len(parts) >= 2 {
					tagNum := parts[1] // e.g. "80:"
					tagClean := strings.TrimSuffix(tagNum, ":")
					fieldName := parts[len(parts)-1]
					featureKey := fmt.Sprintf("%s::Tag_%s (%s)", sym.Name, tagClean, fieldName)
					if seen[featureKey] {
						continue
					}
					seen[featureKey] = true

					// Check if bot references this tag or field
					tagPattern := fmt.Sprintf(", %s,", tagClean)
					tagPattern2 := fmt.Sprintf("Tag(%s,", tagClean)
					tagPattern3 := fmt.Sprintf("appendtag(%s,", tagClean)
					tagPattern4 := fmt.Sprintf(", %s)", tagClean)
					namePattern := strings.ToLower(fieldName)

					if strings.Contains(codeStr, tagPattern) ||
						strings.Contains(codeStr, tagPattern2) ||
						strings.Contains(codeStrLower, tagPattern3) ||
						strings.Contains(codeStr, tagPattern4) ||
						(len(namePattern) > 3 && strings.Contains(codeStrLower, namePattern)) {
						covered = append(covered, featureKey)
					} else {
						missing = append(missing, featureKey)
					}
				}
			}
			continue
		} else if sym.Kind == "game_hunt" {
			// e.g. hunt/monster name or ID
			for _, f := range sym.Fields {
				if strings.HasPrefix(f, "Monster: ") || strings.HasPrefix(f, "Gate: ") {
					parts := strings.Split(f, "(")
					if len(parts) > 1 {
						id := strings.Trim(strings.TrimSuffix(parts[1], ")"), " ")
						if id != "" && !seen[id] {
							seen[id] = true
							checkTokens = append(checkTokens, id)
						}
					}
					// Also check monster names inside e.g. "Monster: Banshee (hunt-40)"
					colonParts := strings.Split(f, ":")
					if len(colonParts) > 1 {
						namePart := strings.Split(colonParts[1], "(")[0]
						for _, mName := range strings.Split(namePart, ",") {
							mClean := strings.TrimSpace(mName)
							if mClean != "" && !seen[mClean] {
								seen[mClean] = true
								checkTokens = append(checkTokens, mClean)
							}
						}
					}
				}
			}
			displayName = sym.Name
			// Also include sym.Receiver and base name (e.g. "Bulbasaur" from "Bulbasaur [Types: ...]")
			baseName := strings.Split(sym.Name, "[")[0]
			baseName = strings.TrimSpace(baseName)
			if baseName != "" {
				checkTokens = append(checkTokens, baseName)
			}
			if sym.Receiver != "" {
				checkTokens = append(checkTokens, sym.Receiver)
			}
		} else {
			cleanName := strings.TrimSpace(sym.Name)
			if cleanName != "" && !seen[cleanName] {
				seen[cleanName] = true
				checkTokens = append(checkTokens, cleanName)
			}
		}

		if len(checkTokens) > 0 {
			isFound := false
			for _, tok := range checkTokens {
				tokLower := strings.ToLower(tok)
				if strings.Contains(codeStrLower, tokLower) {
					isFound = true
					break
				}
			}
			if isFound {
				covered = append(covered, displayName)
			} else {
				missing = append(missing, displayName)
			}
		}
	}

	sort.Strings(covered)
	sort.Strings(missing)

	total := len(covered) + len(missing)
	pct := 0.0
	if total > 0 {
		pct = (float64(len(covered)) / float64(total)) * 100.0
	}

	return &AuditReport{
		TotalGameFeatures: total,
		CoveredFeatures:   len(covered),
		CoveragePercent:   pct,
		CoveredList:       covered,
		MissingList:       missing,
	}, nil
}
