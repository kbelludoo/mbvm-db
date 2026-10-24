package mbvmdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strings"
)

var (
	// MagicBytes identifies an MBVM-DB binary file: 'M', 'B', 'V', 'M'
	MagicBytes = [4]byte{'M', 'B', 'V', 'M'}
	// FileFormatVersion is 1.0 (Major 1, Minor 0)
	FileFormatVersion = [2]byte{0x01, 0x00}
)

var (
	ErrInvalidMagic    = errors.New("mbvmdb: invalid magic header (not an MBVM-DB file)")
	ErrVersionMismatch = errors.New("mbvmdb: unsupported database format version")
	ErrCorruptData     = errors.New("mbvmdb: checksum mismatch, file corrupted")
	ErrEmptyPath       = errors.New("mbvmdb: database file path cannot be empty")
)

// Header represents the on-disk binary header of an MBVM-DB file.
type Header struct {
	Magic           [4]byte
	Version         [2]byte
	Flags           uint16
	TotalFiles      uint32
	TotalRawBytes   int64
	TotalSymbols    uint32
	StringPoolCount uint32
	BytecodeLength  uint32
	PayloadCRC32    uint32
}

// SaveToFile serializes the DB to disk using an atomic write pattern (temp file + rename)
// and computes a CRC32 checksum for corruption detection.
func (db *DB) SaveToFile(filename string) error {
	if filename == "" {
		return ErrEmptyPath
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	// 1. Ensure all symbol strings are pre-interned in db.pool before writing the pool
	for _, sym := range db.symbols {
		db.pool.Intern(sym.Kind)
		db.pool.Intern(sym.Package)
		db.pool.Intern(sym.Name)
		db.pool.Intern(sym.Receiver)
		db.pool.Intern(sym.Signature)
		for _, f := range sym.Fields {
			db.pool.Intern(f)
		}
		for _, m := range sym.Methods {
			db.pool.Intern(m)
		}
		for _, c := range sym.OutgoingCalls {
			db.pool.Intern(c)
		}
		for _, c := range sym.IncomingCalls {
			db.pool.Intern(c)
		}
	}

	// 2. Serialize the payload into an in-memory buffer
	var payload bytes.Buffer

	// Write StringPool
	stringCount := uint32(len(db.pool.pool))
	if err := binary.Write(&payload, binary.BigEndian, stringCount); err != nil {
		return fmt.Errorf("failed to write string count: %w", err)
	}
	for _, s := range db.pool.pool {
		sBytes := []byte(s)
		sLen := uint16(len(sBytes))
		if err := binary.Write(&payload, binary.BigEndian, sLen); err != nil {
			return fmt.Errorf("failed to write string length: %w", err)
		}
		if _, err := payload.Write(sBytes); err != nil {
			return fmt.Errorf("failed to write string bytes: %w", err)
		}
	}

	// Helper to write string id
	writeStrID := func(buf *bytes.Buffer, s string) error {
		id := db.pool.Intern(s)
		return binary.Write(buf, binary.BigEndian, id)
	}

	// Write Symbols
	symbolsCount := uint32(len(db.symbols))
	if err := binary.Write(&payload, binary.BigEndian, symbolsCount); err != nil {
		return fmt.Errorf("failed to write symbols count: %w", err)
	}

	for _, sym := range db.symbols {
		if err := binary.Write(&payload, binary.BigEndian, sym.ID); err != nil {
			return err
		}
		if err := writeStrID(&payload, sym.Kind); err != nil {
			return err
		}
		if err := writeStrID(&payload, sym.Package); err != nil {
			return err
		}
		if err := writeStrID(&payload, sym.Name); err != nil {
			return err
		}
		if err := writeStrID(&payload, sym.Receiver); err != nil {
			return err
		}
		if err := writeStrID(&payload, sym.Signature); err != nil {
			return err
		}
		if err := binary.Write(&payload, binary.BigEndian, sym.BytecodeOffset); err != nil {
			return err
		}
		if err := binary.Write(&payload, binary.BigEndian, sym.BytecodeLen); err != nil {
			return err
		}

		// Fields
		if err := binary.Write(&payload, binary.BigEndian, uint16(len(sym.Fields))); err != nil {
			return err
		}
		for _, f := range sym.Fields {
			if err := writeStrID(&payload, f); err != nil {
				return err
			}
		}

		// Methods
		if err := binary.Write(&payload, binary.BigEndian, uint16(len(sym.Methods))); err != nil {
			return err
		}
		for _, m := range sym.Methods {
			if err := writeStrID(&payload, m); err != nil {
				return err
			}
		}

		// Outgoing Calls
		if err := binary.Write(&payload, binary.BigEndian, uint16(len(sym.OutgoingCalls))); err != nil {
			return err
		}
		for _, call := range sym.OutgoingCalls {
			if err := writeStrID(&payload, call); err != nil {
				return err
			}
		}

		// Incoming Calls
		if err := binary.Write(&payload, binary.BigEndian, uint16(len(sym.IncomingCalls))); err != nil {
			return err
		}
		for _, call := range sym.IncomingCalls {
			if err := writeStrID(&payload, call); err != nil {
				return err
			}
		}
	}

	// Write Bytecode
	bytecodeLen := uint32(len(db.bytecode))
	if err := binary.Write(&payload, binary.BigEndian, bytecodeLen); err != nil {
		return fmt.Errorf("failed to write bytecode length: %w", err)
	}
	if _, err := payload.Write(db.bytecode); err != nil {
		return fmt.Errorf("failed to write bytecode bytes: %w", err)
	}

	// 3. Compute CRC32 checksum over the payload
	payloadBytes := payload.Bytes()
	crc := crc32.ChecksumIEEE(payloadBytes)

	// 4. Prepare Header
	hdr := Header{
		Magic:           MagicBytes,
		Version:         FileFormatVersion,
		Flags:           0,
		TotalFiles:      uint32(db.TotalFiles),
		TotalRawBytes:   db.TotalRawBytes,
		TotalSymbols:    symbolsCount,
		StringPoolCount: stringCount,
		BytecodeLength:  bytecodeLen,
		PayloadCRC32:    crc,
	}

	// 5. Write Header + Payload to temporary file for atomic replace
	tmpFilename := filename + ".tmp"
	f, err := os.OpenFile(tmpFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp db file: %w", err)
	}

	headerErr := binary.Write(f, binary.BigEndian, &hdr)
	if headerErr != nil {
		_ = f.Close()
		_ = os.Remove(tmpFilename)
		return fmt.Errorf("failed to write header: %w", headerErr)
	}

	if _, err := f.Write(payloadBytes); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFilename)
		return fmt.Errorf("failed to write payload: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFilename)
		return fmt.Errorf("failed to sync db file: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpFilename)
		return fmt.Errorf("failed to close temp db file: %w", err)
	}

	// 6. Atomic Rename (on Windows, os.Rename fails if target exists, so remove target first)
	_ = os.Remove(filename)
	if err := os.Rename(tmpFilename, filename); err != nil {
		return fmt.Errorf("failed to atomically rename db file: %w", err)
	}

	return nil
}

// LoadFromFile loads an MBVM-DB instance from a binary file on disk,
// validating the magic header, format version, and CRC32 checksum.
func LoadFromFile(filename string) (*DB, error) {
	if filename == "" {
		return nil, ErrEmptyPath
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hdr Header
	if err := binary.Read(f, binary.BigEndian, &hdr); err != nil {
		return nil, fmt.Errorf("failed to read db header: %w", err)
	}

	if hdr.Magic != MagicBytes {
		return nil, ErrInvalidMagic
	}

	if hdr.Version[0] != FileFormatVersion[0] {
		return nil, ErrVersionMismatch
	}

	// Read remainder of file (payload)
	payloadBytes, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read db payload: %w", err)
	}

	// Validate CRC32
	crc := crc32.ChecksumIEEE(payloadBytes)
	if crc != hdr.PayloadCRC32 {
		return nil, ErrCorruptData
	}

	reader := bytes.NewReader(payloadBytes)

	// Deserialize StringPool
	var stringCount uint32
	if err := binary.Read(reader, binary.BigEndian, &stringCount); err != nil {
		return nil, fmt.Errorf("failed to read string count: %w", err)
	}

	pool := NewStringPool()
	for i := uint32(0); i < stringCount; i++ {
		var sLen uint16
		if err := binary.Read(reader, binary.BigEndian, &sLen); err != nil {
			return nil, fmt.Errorf("failed to read string length: %w", err)
		}
		sBuf := make([]byte, sLen)
		if _, err := io.ReadFull(reader, sBuf); err != nil {
			return nil, fmt.Errorf("failed to read string bytes: %w", err)
		}
		pool.Intern(string(sBuf))
	}

	readStr := func(r io.Reader) (string, error) {
		var id uint16
		if err := binary.Read(r, binary.BigEndian, &id); err != nil {
			return "", err
		}
		return pool.Get(id), nil
	}

	// Deserialize Symbols
	var symbolsCount uint32
	if err := binary.Read(reader, binary.BigEndian, &symbolsCount); err != nil {
		return nil, fmt.Errorf("failed to read symbols count: %w", err)
	}

	symbols := make([]*SymbolRecord, 0, symbolsCount)
	indexByName := make(map[string]*SymbolRecord, symbolsCount*2)
	methodsByReceiver := make(map[string][]*SymbolRecord, 64)

	for i := uint32(0); i < symbolsCount; i++ {
		var sym SymbolRecord
		if err := binary.Read(reader, binary.BigEndian, &sym.ID); err != nil {
			return nil, err
		}
		var err error
		if sym.Kind, err = readStr(reader); err != nil {
			return nil, err
		}
		if sym.Package, err = readStr(reader); err != nil {
			return nil, err
		}
		if sym.Name, err = readStr(reader); err != nil {
			return nil, err
		}
		if sym.Receiver, err = readStr(reader); err != nil {
			return nil, err
		}
		if sym.Signature, err = readStr(reader); err != nil {
			return nil, err
		}
		if err := binary.Read(reader, binary.BigEndian, &sym.BytecodeOffset); err != nil {
			return nil, err
		}
		if err := binary.Read(reader, binary.BigEndian, &sym.BytecodeLen); err != nil {
			return nil, err
		}

		// Fields
		var fieldsCount uint16
		if err := binary.Read(reader, binary.BigEndian, &fieldsCount); err != nil {
			return nil, err
		}
		sym.Fields = make([]string, fieldsCount)
		for j := uint16(0); j < fieldsCount; j++ {
			if sym.Fields[j], err = readStr(reader); err != nil {
				return nil, err
			}
		}

		// Methods
		var methodsCount uint16
		if err := binary.Read(reader, binary.BigEndian, &methodsCount); err != nil {
			return nil, err
		}
		sym.Methods = make([]string, methodsCount)
		for j := uint16(0); j < methodsCount; j++ {
			if sym.Methods[j], err = readStr(reader); err != nil {
				return nil, err
			}
		}

		// Outgoing Calls
		var outCount uint16
		if err := binary.Read(reader, binary.BigEndian, &outCount); err != nil {
			return nil, err
		}
		sym.OutgoingCalls = make([]string, outCount)
		for j := uint16(0); j < outCount; j++ {
			if sym.OutgoingCalls[j], err = readStr(reader); err != nil {
				return nil, err
			}
		}

		// Incoming Calls
		var inCount uint16
		if err := binary.Read(reader, binary.BigEndian, &inCount); err != nil {
			return nil, err
		}
		sym.IncomingCalls = make([]string, inCount)
		for j := uint16(0); j < inCount; j++ {
			if sym.IncomingCalls[j], err = readStr(reader); err != nil {
				return nil, err
			}
		}

		rec := &sym
		symbols = append(symbols, rec)
		indexByName[rec.Name] = rec
		indexByName[strings.ToLower(rec.Name)] = rec
		if rec.Package != "" {
			indexByName[fmt.Sprintf("%s.%s", rec.Package, rec.Name)] = rec
			indexByName[strings.ToLower(fmt.Sprintf("%s.%s", rec.Package, rec.Name))] = rec
		}
		if rec.Receiver != "" {
			cleanRecv := CleanReceiverName(rec.Receiver)
			indexByName[fmt.Sprintf("%s.%s", cleanRecv, rec.Name)] = rec
			indexByName[strings.ToLower(fmt.Sprintf("%s.%s", cleanRecv, rec.Name))] = rec
			methodsByReceiver[cleanRecv] = append(methodsByReceiver[cleanRecv], rec)
		}

		// Re-index game hunt aliases and proto tags stored in signature/fields
		if rec.Kind == "game_hunt" {
			for _, f := range rec.Fields {
				if strings.HasPrefix(f, "Monster: ") {
					// e.g. "Monster: Rato da Ruína (rato-ruina)"
					parts := strings.Split(f, "(")
					if len(parts) > 1 {
						mID := strings.Trim(strings.TrimSuffix(parts[1], ")"), " ")
						indexByName[mID] = rec
					}
				}
			}
		}
	}

	// Deserialize Bytecode
	var bytecodeLen uint32
	if err := binary.Read(reader, binary.BigEndian, &bytecodeLen); err != nil {
		return nil, fmt.Errorf("failed to read bytecode length: %w", err)
	}

	bytecode := make([]byte, bytecodeLen)
	if _, err := io.ReadFull(reader, bytecode); err != nil {
		return nil, fmt.Errorf("failed to read bytecode bytes: %w", err)
	}

	db := &DB{
		pool:              pool,
		symbols:           symbols,
		indexByName:       indexByName,
		methodsByReceiver: methodsByReceiver,
		bytecode:          bytecode,
		TotalFiles:        int(hdr.TotalFiles),
		TotalRawBytes:     hdr.TotalRawBytes,
	}

	db.buildCSR()
	return db, nil
}
