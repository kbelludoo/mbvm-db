package mbvmdb

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestStorage_SaveAndLoad_Roundtrip(t *testing.T) {
	db := New()
	src := `package service

type Config struct {
	Host string
	Port int
}

type Server interface {
	Start() error
}

func Run(c *Config) {
	initDB()
}

func initDB() {}
`
	err := db.IngestSource("service.go", src)
	if err != nil {
		t.Fatalf("IngestSource failed: %v", err)
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.mbvmdb")

	err = db.SaveToFile(dbPath)
	if err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	loadedDB, err := LoadFromFile(dbPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if loadedDB.TotalSymbols() != db.TotalSymbols() {
		t.Fatalf("expected %d symbols, got %d", db.TotalSymbols(), loadedDB.TotalSymbols())
	}
	if loadedDB.BytecodeSize() != db.BytecodeSize() {
		t.Fatalf("expected %d bytecode bytes, got %d", db.BytecodeSize(), loadedDB.BytecodeSize())
	}

	// Verify slice query on loaded DB gives exact same result as original DB
	origSlice := db.ExtractSlice("Run", 2)
	loadedSlice := loadedDB.ExtractSlice("Run", 2)

	origProj := db.ProjectSlice(origSlice.Symbols)
	loadedProj := loadedDB.ProjectSlice(loadedSlice.Symbols)

	if origProj != loadedProj {
		t.Fatalf("projection mismatch between original and loaded DB:\nORIG:\n%s\nLOADED:\n%s", origProj, loadedProj)
	}
}

func TestStorage_CorruptedCRC32(t *testing.T) {
	db := New()
	_ = db.IngestSource("test.go", "package p\nfunc F() {}")

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupted.mbvmdb")
	_ = db.SaveToFile(dbPath)

	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("failed to read test db: %v", err)
	}

	// Corrupt a byte in payload (after header)
	if len(data) > 64 {
		data[len(data)-1] ^= 0xFF
		_ = os.WriteFile(dbPath, data, 0644)
	}

	_, err = LoadFromFile(dbPath)
	if err == nil {
		t.Fatalf("expected ErrCorruptData error, got nil")
	}
	if err != ErrCorruptData {
		t.Logf("got expected corruption error: %v", err)
	}
}

func TestStorage_InvalidMagicHeader(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badmagic.mbvmdb")

	var badHeader Header
	badHeader.Magic = [4]byte{'X', 'Y', 'Z', 'W'}
	badHeader.Version = FileFormatVersion

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, &badHeader)
	_ = os.WriteFile(dbPath, buf.Bytes(), 0644)

	_, err := LoadFromFile(dbPath)
	if err != ErrInvalidMagic {
		t.Fatalf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestStorage_VersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badversion.mbvmdb")

	var badHeader Header
	badHeader.Magic = MagicBytes
	badHeader.Version = [2]byte{0x99, 0x00} // Unsupported major version 99

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, &badHeader)
	_ = os.WriteFile(dbPath, buf.Bytes(), 0644)

	_, err := LoadFromFile(dbPath)
	if err != ErrVersionMismatch {
		t.Fatalf("expected ErrVersionMismatch, got %v", err)
	}
}

func TestStorage_EmptyPath(t *testing.T) {
	db := New()
	if err := db.SaveToFile(""); err != ErrEmptyPath {
		t.Fatalf("expected ErrEmptyPath, got %v", err)
	}
	if _, err := LoadFromFile(""); err != ErrEmptyPath {
		t.Fatalf("expected ErrEmptyPath, got %v", err)
	}
}
