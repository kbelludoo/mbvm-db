package mbvm8k2

import (
	"bytes"
	"testing"
)

// FuzzDecodeV2 is deliberately bounded to one MiB per generated input.
// No corrupted or truncated data may cause a panic or unbounded allocation.
func FuzzDecodeV2(f *testing.F) {
	var valid bytes.Buffer
	if err:=Encode(&valid,Snapshot{
		Symbols:[]Symbol{{Qualified:"pkg.A"},{Qualified:"pkg.B"}},
		Edges:[]Edge{{Caller:0,Callee:1,Position:"a.go:5"}},
	});err!=nil{f.Fatal(err)}
	f.Add(valid.Bytes())
	f.Add([]byte("MBVM8K2\x00"))
	f.Add([]byte("MBVM\x01\x00"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw)>1<<20 {t.Skip()}
		_,_ = Decode(bytes.NewReader(raw),int64(len(raw)),
			Limits{MaxFileBytes:1<<20,MaxSymbols:10000,MaxEdges:100000,MaxStrings:100000})
	})
}
