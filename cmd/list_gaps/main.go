package main

import (
	"fmt"
	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func main() {
	rep, err := mbvmdb.AuditBotCoverage("c:/Users/kbell/Downloads/bot go sigma")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Printf("Total Missing: %d\n", len(rep.MissingList))
	for i, m := range rep.MissingList {
		if i < 20 {
			fmt.Println(m)
		}
	}
}
