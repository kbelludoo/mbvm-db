package main

import (
	"fmt"
	"os"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]
	switch cmd {
	case "slice", "query":
		if len(os.Args) < 4 {
			fmt.Println("Uso: mbvm-db slice <dir_projeto> <simbolo_ou_funcao> [profundidade]")
			os.Exit(1)
		}
		dir := os.Args[2]
		sym := os.Args[3]
		depth := 2
		if len(os.Args) > 4 {
			_, _ = fmt.Sscanf(os.Args[4], "%d", &depth)
		}

		db := mbvmdb.New()
		if err := db.IngestGoAST(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao indexar: %v\n", err)
			os.Exit(1)
		}

		slice := db.ExtractSlice(sym, depth)
		fmt.Fprintf(os.Stderr, "⚡ [MBVM-DB] Símbolo '%s' fatiado com profundidade %d (%d símbolos, ~%d tokens)\n\n",
			sym, depth, len(slice.Symbols), slice.TokensEstimate)
		fmt.Println(db.ProjectSlice(slice.Symbols))

	case "inspect":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		db := mbvmdb.New()
		if err := db.IngestGoAST(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao inspecionar: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("📦 MBVM-DB Status: %s\n", dir)
		fmt.Printf("  • Total de Arquivos: %d\n", db.TotalFiles)
		fmt.Printf("  • Total de Símbolos Indexados: %d\n", db.TotalSymbols())
		fmt.Printf("  • Tamanho do Bytecode Compilado: %d bytes\n", db.BytecodeSize())

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("MBVM-DB — Micro-Bytecode Virtual Machine Database Engine (Go 1.27)")
	fmt.Println()
	fmt.Println("COMANDOS:")
	fmt.Println("  mbvm-db slice <dir> <simbolo> [profundidade]    Extrai fatia subgrafo para IA (-93% tokens sobre MB-VM puro)")
	fmt.Println("  mbvm-db inspect [dir]                           Inspeciona contagem de símbolos e bytecode do projeto")
}
