package mbvmdb

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGameKnowledgeIngestionAndSlice(t *testing.T) {
	db := New()

	protoFiles := []string{
		`c:/Users/kbell/Downloads/bot leveling go/proto_part_0.bin`,
		`c:/Users/kbell/Downloads/bot leveling go/proto_part_4.bin`,
		`c:/Users/kbell/Downloads/bot leveling go/leveling_v1_character.proto.bin`,
	}

	for _, pf := range protoFiles {
		if err := db.IngestProtoDescriptor(pf); err != nil {
			t.Fatalf("failed to ingest %s: %v", pf, err)
		}
	}
	for _, s := range db.symbols {
		if s.Kind == "proto_message" {
			t.Logf("Ingested msg: %s (pkg: %s)", s.Name, s.Package)
		}
	}

	catalogEntries := []GameCatalogItem{
		{
			GateID:        "blue-gate",
			GateName:      "Blue Gate",
			TierMin:       1,
			TierMax:       5,
			MonsterID:     "rato-ruina",
			MonsterName:   "Rato da Ruína",
			BossName:      "Rato Gigante",
			Difficulty:    "normal",
			AreaLevel:     5,
			RequiredLevel: 1,
		},
		{
			GateID:        "castelo-demonios",
			GateName:      "Castelo dos Demônios",
			TierMin:       21,
			TierMax:       25,
			MonsterID:     "orc-jovem",
			MonsterName:   "Orc Jovem",
			BossName:      "Orc Senhor da Guerra",
			Difficulty:    "normal",
			AreaLevel:     25,
			RequiredLevel: 21,
		},
	}
	db.IngestCatalogEntries(catalogEntries)
	db.CompileGameBytecode()

	// 1. Query Proto Message
	sliceClient := db.ExtractSlice("ClientMessage", 1)
	if len(sliceClient.Symbols) == 0 {
		t.Fatalf("expected ClientMessage symbol, got none")
	}
	projClient := db.ProjectSlice(sliceClient.Symbols)
	if !strings.Contains(projClient, "message ClientMessage") {
		t.Fatalf("expected ClientMessage in projection, got: %s", projClient)
	}

	// 2. Query Game Monster by ID
	sliceMonster := db.ExtractSlice("rato-ruina", 1)
	if len(sliceMonster.Symbols) == 0 {
		t.Fatalf("expected rato-ruina monster, got none")
	}
	projMonster := db.ProjectSlice(sliceMonster.Symbols)
	if !strings.Contains(projMonster, "Rato da Ruína") {
		t.Fatalf("expected Rato da Ruína in projection, got: %s", projMonster)
	}

	// 3. Save to temp and verify load
	tmpPath := filepath.Join(t.TempDir(), "game.mbvmdb")
	if err := db.SaveToFile(tmpPath); err != nil {
		t.Fatalf("failed to save game.mbvmdb: %v", err)
	}

	loadedDB, err := LoadFromFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to load game.mbvmdb: %v", err)
	}

	loadedSlice := loadedDB.ExtractSlice("rato-ruina", 1)
	if len(loadedSlice.Symbols) == 0 {
		t.Fatalf("expected rato-ruina in reloaded DB")
	}
}
