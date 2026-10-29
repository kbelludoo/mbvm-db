package mbvmdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChessIngestAndSlice(t *testing.T) {
	db := New()

	openings := []ChessOpeningItem{
		{
			ECO:           "B90",
			Name:          "Sicilian Defense: Najdorf Variation",
			Moves:         "1. e4 c5 2. Nf3 d6 3. d4 cxd4 4. Nxd4 Nf6 5. Nc3 a6",
			WhiteWin:      38.5,
			DrawRate:      31.5,
			BlackWin:      30.0,
			Continuations: []string{"6. Be3", "6. Bg5", "6. Be2", "6. f4", "6. h3"},
			KeyThemes:     []string{"Pawn break ...d5", "Expansion ...b5", "Counterattack on semi-open c-file"},
		},
		{
			ECO:           "C50",
			Name:          "Italian Game: Giuoco Piano",
			Moves:         "1. e4 e5 2. Nf3 Nc6 3. Bc4 Bc5",
			WhiteWin:      39.0,
			DrawRate:      33.0,
			BlackWin:      28.0,
			Continuations: []string{"4. c3", "4. d3", "4. O-O", "4. b4"},
			KeyThemes:     []string{"Control d4 square", "Target f7 square", "Solid pawn structure"},
		},
	}

	db.IngestChessOpenings(openings)

	pgn := `[Event "Live Chess"]
[Site "Chess.com"]
[Date "2026.10.24"]
[White "Hikaru"]
[Black "MagnusCarlsen"]
[Result "1-0"]
[WhiteElo "2875"]
[BlackElo "2882"]
[ECO "B90"]
[Opening "Sicilian Defense: Najdorf"]
[TimeControl "180+2"]

1. e4 c5 2. Nf3 d6 3. d4 cxd4 4. Nxd4 Nf6 5. Nc3 a6 6. Be3 e5 7. Nb3 Be6 8. f3 h5 9. Qd2 Nbd7 10. O-O-O Be7 1-0`

	if err := db.IngestChessPGN(pgn); err != nil {
		t.Fatalf("Failed to ingest chess PGN: %v", err)
	}

	db.CompileChessBytecode()

	if db.TotalSymbols() < 3 {
		t.Fatalf("Expected at least 3 symbols, got %d", db.TotalSymbols())
	}

	// Test slice by ECO code
	sliceECO := db.ExtractSlice("B90", 1)
	if len(sliceECO.Symbols) == 0 {
		t.Errorf("Expected slice for B90 to return symbols")
	}

	// Test slice by Opening name
	sliceNajdorf := db.ExtractSlice("Najdorf", 1)
	if len(sliceNajdorf.Symbols) == 0 {
		t.Errorf("Expected slice for 'Najdorf' to return symbols")
	}

	// Test slice by move
	sliceMove := db.ExtractSlice("e4", 1)
	if len(sliceMove.Symbols) == 0 {
		t.Errorf("Expected slice for move 'e4' to return symbols")
	}

	// Test save and load binary
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test_chess.mbvmdb")

	if err := db.SaveToFile(outPath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	loadedDB, err := LoadFromFile(outPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if loadedDB.TotalSymbols() != db.TotalSymbols() {
		t.Errorf("Loaded DB symbols %d != original %d", loadedDB.TotalSymbols(), db.TotalSymbols())
	}

	// Verify slice on loaded DB
	sliceAfterLoad := loadedDB.ExtractSlice("B90", 1)
	if len(sliceAfterLoad.Symbols) == 0 {
		t.Errorf("Expected slice from loaded DB to return symbols")
	}

	_ = os.Remove(outPath)
}
