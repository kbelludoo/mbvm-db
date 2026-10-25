package mbvm8k2

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"hash/crc32"
)

type outputEdge struct {
	Edge
	positionID uint32
}

func canonicalize(s Snapshot) ([]Symbol, []outputEdge, []string, map[string]uint32, error) {
	limits := (Limits{}).normalized()
	if uint64(len(s.Symbols)) > limits.MaxSymbols || uint64(len(s.Edges)) > limits.MaxEdges {
		return nil,nil,nil,nil,ErrTooLarge
	}
	rows := append([]Symbol(nil), s.Symbols...)
	order := make([]int, len(rows))
	for i := range order { order[i] = i }
	sort.Slice(order, func(i,j int) bool {
		return rows[order[i]].Qualified < rows[order[j]].Qualified
	})
	remap := make([]uint32,len(rows))
	sorted := make([]Symbol,len(rows))
	stringsSet := map[string]bool{"":true}
	addString := func(value string) error {
		if !wellFormedText(value) { return invalid("invalid UTF-8 or NUL in string") }
		stringsSet[value] = true
		return nil
	}
	for idx, old := range order {
		v := rows[old]
		if v.Qualified=="" || (idx>0 && sorted[idx-1].Qualified==v.Qualified) {
			return nil,nil,nil,nil,invalid("empty or duplicate qualified symbol identity")
		}
		if v.BytecodeLength==0 && v.BytecodeOffset!=0 {
			return nil,nil,nil,nil,invalid("bytecode offset without length")
		}
		if v.BytecodeLength>0 &&
			(len(s.Bytecode)==0 || !checkedRange(v.BytecodeOffset,v.BytecodeLength,uint64(len(s.Bytecode)))) {
			return nil,nil,nil,nil,invalid("symbol bytecode range out of bounds")
		}
		for _, field := range []string{v.Qualified,v.Name,v.Package,v.Receiver,v.Kind,v.Signature} {
			if err:=addString(field); err!=nil { return nil,nil,nil,nil,err }
		}
		remap[old]=uint32(idx)
		sorted[idx]=v
	}
	edges := make([]outputEdge,len(s.Edges))
	for i,e := range s.Edges {
		if uint64(e.Caller)>=uint64(len(rows)) || uint64(e.Callee)>=uint64(len(rows)) {
			return nil,nil,nil,nil,invalid("edge references absent symbol")
		}
		if err:=addString(e.Position);err!=nil{return nil,nil,nil,nil,err}
		edges[i]=outputEdge{Edge: Edge{
			Caller: remap[e.Caller], Callee: remap[e.Callee],
			Position: e.Position, CrossPackage:e.CrossPackage,
		}}
	}
	if uint64(len(stringsSet))>limits.MaxStrings {return nil,nil,nil,nil,ErrTooLarge}
	pool:=make([]string,0,len(stringsSet))
	for v:=range stringsSet{pool=append(pool,v)}
	sort.Strings(pool)
	if len(pool)==0 || pool[0]!="" {return nil,nil,nil,nil,invalid("string ID 0 must be empty")}
	stringIDs:=make(map[string]uint32,len(pool))
	for i,v:=range pool{stringIDs[v]=uint32(i)}
	for i:=range edges{edges[i].positionID=stringIDs[edges[i].Position]}
	sort.Slice(edges,func(i,j int) bool {
		a,b:=edges[i],edges[j]
		if a.Caller!=b.Caller{return a.Caller<b.Caller}
		if a.Callee!=b.Callee{return a.Callee<b.Callee}
		if a.positionID!=b.positionID{return a.positionID<b.positionID}
		return !a.CrossPackage && b.CrossPackage
	})
	return sorted,edges,pool,stringIDs,nil
}

func buildSections(s Snapshot) ([]wireSection,uint64,error) {
	symbols,edges,pool,ids,err:=canonicalize(s)
	if err!=nil{return nil,0,err}
	sections:=make([]wireSection,0,7)
	blob:=make([]byte,0)
	positions:=make([]uint32,0,len(pool)+1)
	var length uint64
	for _,v:=range pool {
		if length+uint64(len(v))>uint64(^uint32(0)) {
			return nil,0,ErrTooLarge
		}
		positions=append(positions,uint32(length))
		length+=uint64(len(v))
		blob=append(blob,v...)
	}
	positions=append(positions,uint32(length))
	data:=make([]byte,0,4*len(positions)+len(blob))
	for _,v:=range positions{data=add32(data,v)}
	data=append(data,blob...)
	sections=append(sections,wireSection{id:sectionStrings,content:data,count:uint64(len(pool))})
	data=make([]byte,0,48*len(symbols))
	for _,v:=range symbols {
		for _,name:=range []string{v.Qualified,v.Name,v.Package,v.Receiver,v.Kind,v.Signature} {
			data=add32(data,ids[name])
		}
		data=add32(data,0) // flags
		data=add32(data,0) // reserved
		data=add64(data,v.BytecodeOffset)
		data=add64(data,v.BytecodeLength)
	}
	sections=append(sections,wireSection{id:sectionSymbols,content:data,count:uint64(len(symbols)),width:48})
	outOffsets:=make([]uint32,len(symbols)+1)
	for _,e:=range edges{outOffsets[e.Caller+1]++}
	for i:=1;i<len(outOffsets);i++{outOffsets[i]+=outOffsets[i-1]}
	data=make([]byte,0,4*len(outOffsets))
	for _,v:=range outOffsets{data=add32(data,v)}
	sections=append(sections,wireSection{id:sectionOutOffset,content:data,count:uint64(len(outOffsets)),width:4})
	data=make([]byte,0,16*len(edges))
	back:=make([][]uint32,len(symbols))
	for id,e:=range edges {
		data=add32(data,e.Caller)
		data=add32(data,e.Callee)
		data=add32(data,e.positionID)
		var flags uint32
		if e.CrossPackage{flags=1}
		data=add32(data,flags)
		back[e.Callee]=append(back[e.Callee],uint32(id))
	}
	sections=append(sections,wireSection{id:sectionOutEdges,content:data,count:uint64(len(edges)),width:16})
	inOffsets:=make([]uint32,len(symbols)+1)
	for i,row:=range back{inOffsets[i+1]=inOffsets[i]+uint32(len(row))}
	data=make([]byte,0,4*len(inOffsets))
	for _,v:=range inOffsets{data=add32(data,v)}
	sections=append(sections,wireSection{id:sectionInOffset,content:data,count:uint64(len(inOffsets)),width:4})
	data=make([]byte,0,4*len(edges))
	for _,row:=range back {for _,id:=range row {data=add32(data,id)}}
	sections=append(sections,wireSection{id:sectionInEdges,content:data,count:uint64(len(edges)),width:4})
	if len(s.Bytecode)>0 {
		sections=append(sections,wireSection{id:sectionBytecode,flags:1,
			content:append([]byte(nil),s.Bytecode...),count:uint64(len(s.Bytecode))})
	}
	return sections,uint64(len(symbols)),nil
}

// Encode writes one deterministic MBVM8K2 v2.0 snapshot. For now the
// bounded encoder materializes at most 256 MiB; it never changes a v1 file.
func Encode(w io.Writer, snapshot Snapshot) error {
	if w==nil{return invalid("nil writer")}
	sections,symbolCount,err:=buildSections(snapshot)
	if err!=nil{return fmt.Errorf("prepare MBVM8K2 sections: %w",err)}
	cursor:=align8(uint64(headerSize+directoryEntrySize*len(sections)))
	for i:=range sections {
		sections[i].offset=cursor
		cursor=align8(cursor+uint64(len(sections[i].content)))
	}
	// Do not insert padding beyond the final section: end-of-file must
	// coincide with its content boundary.
	last:=sections[len(sections)-1]
	fileLength:=last.offset+uint64(len(last.content))
	if fileLength>uint64((Limits{}).normalized().MaxFileBytes){return ErrTooLarge}
	file:=make([]byte,int(fileLength))
	copy(file[:8],magic[:])
	le.PutUint16(file[8:],2)
	le.PutUint16(file[10:],0)
	le.PutUint32(file[12:],headerSize)
	le.PutUint32(file[20:],uint32(len(sections)))
	le.PutUint64(file[24:],headerSize)
	le.PutUint64(file[32:],fileLength)
	le.PutUint64(file[40:],symbolCount)
	for i,section:=range sections {
		entry:=file[headerSize+i*directoryEntrySize:]
		le.PutUint32(entry[0:],section.id)
		le.PutUint32(entry[4:],section.flags)
		le.PutUint64(entry[8:],section.offset)
		le.PutUint64(entry[16:],uint64(len(section.content)))
		le.PutUint64(entry[24:],section.count)
		le.PutUint32(entry[32:],section.width)
		le.PutUint32(entry[40:],crc32.Checksum(section.content,castagnoli))
		copy(file[section.offset:],section.content)
	}
	hash:=fullFileDigest(file)
	copy(file[48:80],hash[:])
	if _,err:=io.Copy(w,bytes.NewReader(file));err!=nil{return fmt.Errorf("write MBVM8K2: %w",err)}
	return nil
}
