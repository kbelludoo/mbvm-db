package mbvm8k2

import (
	"bytes"
)

func sectionBytes(file []byte, sections map[uint32]decodedSection, id uint32) []byte {
	v:=sections[id]
	return file[v.offset:v.offset+v.length]
}

func readStrings(file []byte, dir map[uint32]decodedSection) ([]string,error) {
	d:=dir[sectionStrings]
	if d.count==0{return nil,invalid("missing empty string ID zero")}
	bytes:=sectionBytes(file,dir,sectionStrings)
	offsetSize:=(d.count+1)*4
	if offsetSize>uint64(len(bytes)){return nil,invalid("truncated string offsets")}
	blob:=bytes[offsetSize:]
	out:=make([]string,int(d.count))
	var previous uint32
	for i:=uint64(0);i<=d.count;i++ {
		current:=le.Uint32(bytes[i*4:])
		if i==0 && current!=0 || uint64(current)>uint64(len(blob)) || current<previous {
			return nil,invalid("invalid string offsets")
		}
		if i>0 {
			value:=string(blob[previous:current])
			if !wellFormedText(value) {return nil,invalid("invalid UTF-8 or NUL in string pool")}
			if i==1 && value!="" || i>1 && value<=out[i-2] {
				return nil,invalid("string table not unique and sorted")
			}
			out[i-1]=value
		}
		previous=current
	}
	if int(previous)!=len(blob){return nil,invalid("undisclosed string-table bytes")}
	return out,nil
}

func readSymbols(file []byte,dir map[uint32]decodedSection,pool []string,n uint64)([]Symbol,error) {
	raw:=sectionBytes(file,dir,sectionSymbols)
	out:=make([]Symbol,int(n))
	previous:=""
	for i:=range out {
		r:=raw[48*i:48*(i+1)]
		var fields [6]string
		for j:=range fields {
			id:=le.Uint32(r[4*j:])
			if uint64(id)>=uint64(len(pool)){return nil,invalid("symbol string ID out of bounds")}
			fields[j]=pool[id]
		}
		if fields[0]=="" || i>0 && fields[0]<=previous {
			return nil,invalid("unsorted or duplicate qualified symbol")
		}
		previous=fields[0]
		if le.Uint32(r[24:])!=0 || le.Uint32(r[28:])!=0 {
			return nil,invalid("nonzero symbol flags or reserved bytes")
		}
		o,l:=le.Uint64(r[32:]),le.Uint64(r[40:])
		if l==0 && o!=0{return nil,invalid("zero-bytecode record has offset")}
		out[i]=Symbol{
			Qualified:fields[0],Name:fields[1],Package:fields[2],
			Receiver:fields[3],Kind:fields[4],Signature:fields[5],
			BytecodeOffset:o,BytecodeLength:l,
		}
	}
	if bytecode,exists:=dir[sectionBytecode];exists {
		for _,v:=range out{
			if v.BytecodeLength>0 && !checkedRange(v.BytecodeOffset,v.BytecodeLength,bytecode.length) {
				return nil,invalid("bytecode extent outside section")
			}
		}
	}else {
		for _,v:=range out{if v.BytecodeLength>0{return nil,invalid("missing bytecode section")}}
	}
	return out,nil
}

func readCSRIndices(raw []byte, expected uint64, edgeCount uint64)([]uint32,error) {
	if expected>uint64(len(raw))/4 || uint64(len(raw))!=4*expected {
		return nil,invalid("CSR offset-array byte length mismatch")
	}
	offsets:=make([]uint32,int(expected))
	var last uint32
	for i:=range offsets {
		v:=le.Uint32(raw[4*i:])
		if i==0 && v!=0 || v<last || uint64(v)>edgeCount{
			return nil,invalid("invalid CSR row offset")
		}
		offsets[i]=v
		last=v
	}
	if uint64(last)!=edgeCount{return nil,invalid("CSR last offset disagrees with E")}
	return offsets,nil
}

func readEdges(file []byte,dir map[uint32]decodedSection,
	pool []string,n uint64)([]Edge,error) {
	edgeSection:=dir[sectionOutEdges]
	e:=edgeSection.count
	edgeRaw:=sectionBytes(file,dir,sectionOutEdges)
	outOffsets,err:=readCSRIndices(sectionBytes(file,dir,sectionOutOffset),n+1,e)
	if err!=nil{return nil,err}
	if dir[sectionOutOffset].count!=n+1{return nil,invalid("forward CSR offset count mismatch")}
	out:=make([]Edge,int(e))
	for i:=uint64(0);i<e;i++ {
		r:=edgeRaw[16*i:16*(i+1)]
		caller,callee,pos,flags:=le.Uint32(r[0:]),le.Uint32(r[4:]),
			le.Uint32(r[8:]),le.Uint32(r[12:])
		if uint64(caller)>=n || uint64(callee)>=n || uint64(pos)>=uint64(len(pool)) || flags &^ uint32(1)!=0 {
			return nil,invalid("invalid edge reference or flags")
		}
		out[i]=Edge{Caller:caller,Callee:callee,Position:pool[pos],CrossPackage:flags==1}
	}
	for i:=uint64(0);i<n;i++ {
		start,end:=outOffsets[i],outOffsets[i+1]
		for j:=start;j<end;j++{
			a:=out[j]
			if uint64(a.Caller)!=i{return nil,invalid("edge outside caller CSR row")}
			if j>start{
				prev:=out[j-1]
				if a.Callee<prev.Callee ||
					a.Callee==prev.Callee && a.Position<prev.Position ||
					a.Callee==prev.Callee && a.Position==prev.Position && !a.CrossPackage && prev.CrossPackage {
					return nil,invalid("unsorted outgoing edges")
				}
			}
		}
	}
	if dir[sectionInOffset].count!=n+1 ||
		dir[sectionInEdges].count!=e {
		return nil,invalid("reverse CSR counts disagree")
	}
	backOffsets,err:=readCSRIndices(sectionBytes(file,dir,sectionInOffset),n+1,e)
	if err!=nil{return nil,err}
	backRaw:=sectionBytes(file,dir,sectionInEdges)
	visited:=make([]bool,int(e))
	for row:=uint64(0);row<n;row++{
		var prev uint32
		for slot:=backOffsets[row];slot<backOffsets[row+1];slot++ {
			id:=le.Uint32(backRaw[4*slot:])
			if uint64(id)>=e || visited[id] ||
				uint64(out[id].Callee)!=row ||
				slot>backOffsets[row] && id<=prev {
				return nil,invalid("reverse CSR not an exact sorted permutation")
			}
			visited[id]=true
			prev=id
		}
	}
	for _,ok:=range visited{if !ok{return nil,invalid("missing reverse edge reference")}}
	return out,nil
}

func decodePayload(file []byte,dir map[uint32]decodedSection,n uint64)(Snapshot,error) {
	pool,err:=readStrings(file,dir)
	if err!=nil{return Snapshot{},err}
	symbols,err:=readSymbols(file,dir,pool,n)
	if err!=nil{return Snapshot{},err}
	edges,err:=readEdges(file,dir,pool,n)
	if err!=nil{return Snapshot{},err}
	snapshot:=Snapshot{Symbols:symbols,Edges:edges}
	if bytecode,exists:=dir[sectionBytecode];exists {
		if bytecode.count!=bytecode.length{return Snapshot{},invalid("bytecode section count mismatch")}
		snapshot.Bytecode=bytes.Clone(sectionBytes(file,dir,sectionBytecode))
	}
	return snapshot,nil
}
