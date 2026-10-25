package mbvm8k2

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"unicode/utf8"
)

var le = binary.LittleEndian
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

type wireSection struct {
	id uint32
	flags uint32
	offset uint64
	content []byte
	count uint64
	width uint32
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidFormat, fmt.Sprintf(format, args...))
}

func wellFormedText(s string) bool {
	if !utf8.ValidString(s) { return false }
	for _, r := range s { if r == 0 { return false } }
	return true
}

func align8(n uint64) uint64 { return (n + 7) &^ uint64(7) }

func checkedRange(offset, length, total uint64) bool {
	return offset <= total && length <= total-offset
}

func sectionWidth(id uint32) uint32 {
	switch id {
	case sectionSymbols: return 48
	case sectionOutOffset, sectionInOffset, sectionInEdges: return 4
	case sectionOutEdges: return 16
	default: return 0
	}
}

func sectionIsRequired(id uint32) bool {
	return id >= sectionStrings && id <= sectionInEdges
}

func add32(dst []byte, value uint32) []byte {
	var b [4]byte
	le.PutUint32(b[:],value)
	return append(dst,b[:]...)
}
func add64(dst []byte, value uint64) []byte {
	var b [8]byte
	le.PutUint64(b[:],value)
	return append(dst,b[:]...)
}

func fullFileDigest(bytes []byte) [32]byte {
	d := sha256.New()
	d.Write(bytes[:48])
	var cleared [32]byte
	d.Write(cleared[:])
	d.Write(bytes[80:])
	var result [32]byte
	copy(result[:], d.Sum(nil))
	return result
}
