package mbvm8k2

import (
	"bytes"
	"crypto/subtle"
	"fmt"
	"hash/crc32"
	"io"
)

type decodedSection struct {
	id uint32
	flags uint32
	offset uint64
	length uint64
	count uint64
	width uint32
}

func verifyEnvelope(file []byte, limits Limits) (map[uint32]decodedSection,uint64,error) {
	if len(file)<headerSize{return nil,0,invalid("truncated header")}
	if !bytes.Equal(file[:8],magic[:]) {return nil,0,invalid("invalid MBVM8K2 magic")}
	if le.Uint16(file[8:])!=2 || le.Uint16(file[10:])!=0 {
		return nil,0,invalid("unsupported v2 version")
	}
	if le.Uint32(file[12:])!=headerSize || le.Uint32(file[16:])!=0 {
		return nil,0,invalid("invalid header size or flags")
	}
	count:=le.Uint32(file[20:])
	if count<6 || count>maxSections {return nil,0,invalid("invalid directory section count")}
	if le.Uint64(file[24:])!=headerSize || le.Uint64(file[32:])!=uint64(len(file)) {
		return nil,0,invalid("invalid file length or directory offset")
	}
	symbols:=le.Uint64(file[40:])
	if symbols>limits.MaxSymbols {return nil,0,ErrTooLarge}
	for _,v:=range file[112:128] {if v!=0{return nil,0,invalid("nonzero reserved header")}}
	actual:=fullFileDigest(file)
	if subtle.ConstantTimeCompare(actual[:],file[48:80])!=1 {
		return nil,0,invalid("whole-file SHA-256 mismatch")
	}
	dirEnd:=uint64(headerSize)+uint64(count)*directoryEntrySize
	if dirEnd>uint64(len(file)){return nil,0,invalid("truncated directory")}
	dir:=make(map[uint32]decodedSection,count)
	previousID:=uint32(0)
	previousEnd:=dirEnd
	for i:=uint32(0);i<count;i++ {
		base:=headerSize+int(i)*directoryEntrySize
		raw:=file[base:base+directoryEntrySize]
		id:=le.Uint32(raw[0:])
		flags:=le.Uint32(raw[4:])
		offset,length:=le.Uint64(raw[8:]),le.Uint64(raw[16:])
		items:=le.Uint64(raw[24:])
		width:=le.Uint32(raw[32:])
		if id<=previousID || flags &^ uint32(1)!=0 ||
			le.Uint32(raw[36:])!=0 || le.Uint32(raw[44:])!=0 {
			return nil,0,invalid("noncanonical directory ID, flags or reserved bytes")
		}
		if !sectionIsRequired(id) && id!=sectionBytecode && flags!=1 {
			return nil,0,invalid("unknown mandatory section %d",id)
		}
		if sectionIsRequired(id) && flags!=0 ||
			id==sectionBytecode && flags!=1 {
			return nil,0,invalid("invalid required/optional section flags")
		}
		if id<=sectionBytecode && width!=sectionWidth(id) {
			return nil,0,invalid("wrong record width of section %d",id)
		}
		if offset%8!=0 || offset<align8(dirEnd) ||
			!checkedRange(offset,length,uint64(len(file))) || offset<previousEnd {
			return nil,0,invalid("invalid or overlapping section %d",id)
		}
		for _,padding:=range file[previousEnd:offset] {
			if padding!=0{return nil,0,invalid("nonzero section padding")}
		}
		if crc32.Checksum(file[offset:offset+length],castagnoli)!=le.Uint32(raw[40:]) {
			return nil,0,invalid("CRC32C mismatch in section %d",id)
		}
		if width>0 && (items>uint64(len(file))/uint64(width) ||
			items*uint64(width)!=length) {
			return nil,0,invalid("record length/count mismatch in section %d",id)
		}
		dir[id]=decodedSection{id:id,flags:flags,offset:offset,length:length,count:items,width:width}
		previousID=id
		previousEnd=offset+length
	}
	for id:=sectionStrings;id<=sectionInEdges;id++ {
		if _,ok:=dir[id];!ok{return nil,0,invalid("missing required section %d",id)}
	}
	if previousEnd!=uint64(len(file)){return nil,0,invalid("undisclosed trailing bytes")}
	if dir[sectionSymbols].count!=symbols{return nil,0,invalid("header symbol count differs")}
	if dir[sectionStrings].count>limits.MaxStrings ||
		dir[sectionOutEdges].count>limits.MaxEdges || dir[sectionInEdges].count>limits.MaxEdges {
		return nil,0,ErrTooLarge
	}
	return dir,symbols,nil
}

// Decode validates the entire file (including both CSR directions) before
// exposing any Snapshot value. The bounded memory reader rejects >256 MiB.
func Decode(source io.ReaderAt, length int64, limits Limits) (Snapshot,error) {
	limits=limits.normalized()
	if source==nil || length<0 {return Snapshot{},invalid("nil reader or negative file length")}
	if length>limits.MaxFileBytes{return Snapshot{},ErrTooLarge}
	if length<headerSize{return Snapshot{},invalid("truncated header")}
	file:=make([]byte,int(length))
	if _,err:=io.ReadFull(io.NewSectionReader(source,0,length),file);err!=nil {
		return Snapshot{},fmt.Errorf("read MBVM8K2: %w",err)
	}
	sections,symbolCount,err:=verifyEnvelope(file,limits)
	if err!=nil{return Snapshot{},err}
	result,err:=decodePayload(file,sections,symbolCount)
	if err!=nil{return Snapshot{},fmt.Errorf("decode MBVM8K2 payload: %w",err)}
	return result,nil
}
