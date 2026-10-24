package mbvmdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// OpCodes for Game & Domain Bytecode
const (
	OpProtoPackage byte = 0x30
	OpProtoMessage byte = 0x31
	OpProtoField   byte = 0x32
	OpProtoEnum    byte = 0x33
	OpGameCatalog  byte = 0x34
	OpGameTier     byte = 0x35
)

// IngestProtoDescriptor parses a compiled protobuf FileDescriptorSet or FileDescriptorProto (.bin / .desc)
// and registers messages, fields, tags and types into the MBVM-DB graph.
func (db *DB) IngestProtoDescriptor(filePath string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read proto descriptor %s: %w", filePath, err)
	}

	info, err := os.Stat(filePath)
	if err == nil {
		db.TotalRawBytes += info.Size()
	}
	db.TotalFiles++

	baseName := filepath.Base(filePath)

	// Try FileDescriptorSet first
	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, fds); err == nil && len(fds.File) > 0 {
		for _, file := range fds.File {
			db.indexFileDescriptorProto(file, baseName)
		}
		return nil
	}

	// Try single FileDescriptorProto
	fdp := &descriptorpb.FileDescriptorProto{}
	if err := proto.Unmarshal(data, fdp); err == nil && fdp.GetName() != "" {
		db.indexFileDescriptorProto(fdp, baseName)
		return nil
	}

	return fmt.Errorf("file %s is not a valid protobuf descriptor", filePath)
}

func (db *DB) indexFileDescriptorProto(fdp *descriptorpb.FileDescriptorProto, sourceFile string) {
	pkgName := fdp.GetPackage()
	if pkgName == "" {
		pkgName = "proto"
	}

	// Index Enums
	for _, enum := range fdp.EnumType {
		enumName := enum.GetName()
		var values []string
		for _, val := range enum.Value {
			valStr := fmt.Sprintf("%s = %d", val.GetName(), val.GetNumber())
			values = append(values, valStr)
			tagKey := fmt.Sprintf("EnumVal:%s.%s", enumName, val.GetName())
			db.indexByName[strings.ToLower(tagKey)] = &SymbolRecord{
				ID:      uint16(len(db.symbols)),
				Kind:    "proto_enum_val",
				Package: pkgName,
				Name:    valStr,
			}
		}

		rec := &SymbolRecord{
			ID:      uint16(len(db.symbols)),
			Kind:    "proto_enum",
			Package: pkgName,
			Name:    enumName,
			Fields:  values,
		}
		db.symbols = append(db.symbols, rec)
		db.indexByName[enumName] = rec
		db.indexByName[strings.ToLower(enumName)] = rec
		db.indexByName[fmt.Sprintf("%s.%s", pkgName, enumName)] = rec
		db.indexByName[strings.ToLower(fmt.Sprintf("%s.%s", pkgName, enumName))] = rec
	}

	// Index Messages
	for _, msg := range fdp.MessageType {
		db.indexDescriptorMessage(pkgName, "", msg, sourceFile)
	}
}

func (db *DB) indexDescriptorMessage(pkgName, parentName string, msg *descriptorpb.DescriptorProto, sourceFile string) {
	msgName := msg.GetName()
	if parentName != "" {
		msgName = parentName + "." + msgName
	}

	var fields []string
	for _, f := range msg.Field {
		fNum := f.GetNumber()
		fName := f.GetName()
		fType := f.GetType().String()
		if f.GetTypeName() != "" {
			parts := strings.Split(f.GetTypeName(), ".")
			fType = parts[len(parts)-1]
		}
		fLabel := ""
		if f.Label != nil {
			switch *f.Label {
			case descriptorpb.FieldDescriptorProto_LABEL_REPEATED:
				fLabel = "repeated "
			case descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL:
				fLabel = "optional "
			case descriptorpb.FieldDescriptorProto_LABEL_REQUIRED:
				fLabel = "required "
			}
		}

		fieldRepr := fmt.Sprintf("Tag %d: %s%s %s", fNum, fLabel, fType, fName)
		fields = append(fields, fieldRepr)

		// Direct lookup by tag inside message, e.g. "Envelope.Tag 1" or "Tag 80"
		tagKey := fmt.Sprintf("%s.Tag %d", msgName, fNum)
		tagRec := &SymbolRecord{
			ID:        uint16(len(db.symbols)),
			Kind:      "proto_tag",
			Package:   pkgName,
			Name:      fieldRepr,
			Receiver:  msgName,
			Signature: fmt.Sprintf("Tag %d", fNum),
		}
		db.indexByName[strings.ToLower(tagKey)] = tagRec
		db.indexByName[strings.ToLower(fmt.Sprintf("%s.#%d", msgName, fNum))] = tagRec

		// Global tag match if unique or first seen
		globalTagKey := fmt.Sprintf("tag %d", fNum)
		if _, exists := db.indexByName[globalTagKey]; !exists {
			db.indexByName[globalTagKey] = tagRec
		}
	}

	rec := &SymbolRecord{
		ID:        uint16(len(db.symbols)),
		Kind:      "proto_message",
		Package:   pkgName,
		Name:      msgName,
		Fields:    fields,
		Signature: fmt.Sprintf("Defined in %s", sourceFile),
	}
	db.symbols = append(db.symbols, rec)

	db.indexByName[msgName] = rec
	db.indexByName[strings.ToLower(msgName)] = rec
	db.indexByName[fmt.Sprintf("%s.%s", pkgName, msgName)] = rec
	db.indexByName[strings.ToLower(fmt.Sprintf("%s.%s", pkgName, msgName))] = rec

	// Nested messages
	for _, nested := range msg.NestedType {
		db.indexDescriptorMessage(pkgName, msgName, nested, sourceFile)
	}
}

// IngestGameCatalog parses game entities and bestiary/gates from catalog definitions into the graph.
type GameCatalogItem struct {
	GateID        string `json:"gate_id"`
	GateName      string `json:"gate_name"`
	TierMin       uint32 `json:"tier_min"`
	TierMax       uint32 `json:"tier_max"`
	MonsterID     string `json:"monster_id"`
	MonsterName   string `json:"monster_name"`
	BossName      string `json:"boss_name,omitempty"`
	Difficulty    string `json:"difficulty"`
	AreaLevel     uint32 `json:"area_level"`
	RequiredLevel uint32 `json:"required_level"`
}

// IngestCatalogEntries adds high-efficiency searchable game content records into MBVM-DB.
func (db *DB) IngestCatalogEntries(entries []GameCatalogItem) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, item := range entries {
		var details []string
		details = append(details, fmt.Sprintf("Gate: %s (%s)", item.GateName, item.GateID))
		details = append(details, fmt.Sprintf("Monster: %s (%s)", item.MonsterName, item.MonsterID))
		if item.BossName != "" {
			details = append(details, fmt.Sprintf("Boss: %s", item.BossName))
		}
		details = append(details, fmt.Sprintf("Tiers: %d - %d | Difficulty: %s", item.TierMin, item.TierMax, item.Difficulty))
		details = append(details, fmt.Sprintf("Required Level: %d | Area Level: %d", item.RequiredLevel, item.AreaLevel))

		rec := &SymbolRecord{
			ID:        uint16(len(db.symbols)),
			Kind:      "game_hunt",
			Package:   "catalog",
			Name:      fmt.Sprintf("%s [%s]", item.MonsterName, item.Difficulty),
			Receiver:  item.GateName,
			Signature: fmt.Sprintf("Lv.%d-%d", item.RequiredLevel, item.AreaLevel),
			Fields:    details,
		}
		db.symbols = append(db.symbols, rec)

		// Search aliases
		aliases := []string{
			item.MonsterID,
			strings.ToLower(item.MonsterName),
			strings.ToLower(item.GateID),
			strings.ToLower(item.GateName),
			fmt.Sprintf("%s-%s", item.MonsterID, item.Difficulty),
			fmt.Sprintf("level-%d", item.RequiredLevel),
		}

		for _, alias := range aliases {
			if _, exists := db.indexByName[alias]; !exists {
				db.indexByName[alias] = rec
			}
		}
	}
}

// CompileGameBytecode writes domain entities into the database bytecode buffer.
func (db *DB) CompileGameBytecode() {
	var buf bytes.Buffer
	for _, s := range db.symbols {
		if strings.HasPrefix(s.Kind, "proto_") {
			buf.WriteByte(OpProtoMessage)
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Name))
			_ = binary.Write(&buf, binary.BigEndian, uint16(len(s.Fields)))
			for _, f := range s.Fields {
				buf.WriteByte(OpProtoField)
				_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(f))
			}
		} else if s.Kind == "game_hunt" {
			buf.WriteByte(OpGameTier)
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Name))
			_ = binary.Write(&buf, binary.BigEndian, uint16(len(s.Fields)))
			for _, f := range s.Fields {
				buf.WriteByte(OpGameCatalog)
				_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(f))
			}
		}
	}
	db.bytecode = append(db.bytecode, buf.Bytes()...)
}
