package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]
	switch cmd {
	case "impact":
		if err := runImpact(os.Args[2:],os.Stdout);err!=nil{
			fmt.Fprintf(os.Stderr,"mbvm-db impact: %v\n",err)
			os.Exit(1)
		}

	case "build":
		if len(os.Args) < 3 {
			fmt.Println("Uso: mbvm-db build <dir_projeto> [arquivo_saida.mbvmdb]")
			os.Exit(1)
		}
		srcDir := os.Args[2]
		outPath := "codebase.mbvmdb"
		if len(os.Args) > 3 {
			outPath = os.Args[3]
		}

		start := time.Now()
		db := mbvmdb.New()
		if err := db.IngestGoAST(srcDir); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao indexar AST: %v\n", err)
			os.Exit(1)
		}

		if err := db.SaveToFile(outPath); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao salvar arquivo binário: %v\n", err)
			os.Exit(1)
		}
		duration := time.Since(start)

		fileInfo, _ := os.Stat(outPath)
		var fileSize int64
		if fileInfo != nil {
			fileSize = fileInfo.Size()
		}

		fmt.Printf("⚡ [MBVM-DB] Banco de dados compilado com sucesso em %v!\n", duration)
		fmt.Printf("  • Arquivo de Saída: %s (%d bytes)\n", outPath, fileSize)
		fmt.Printf("  • Arquivos Processados: %d\n", db.TotalFiles)
		fmt.Printf("  • Símbolos Indexados: %d\n", db.TotalSymbols())
		fmt.Printf("  • Bytecode Compilado: %d bytes\n", db.BytecodeSize())

	case "ingest-proto":
		if len(os.Args) < 4 {
			fmt.Println("Uso: mbvm-db ingest-proto <arquivo_saida.mbvmdb> <proto_file_1.bin> [proto_file_2.bin ...]")
			os.Exit(1)
		}
		outPath := os.Args[2]
		protoFiles := os.Args[3:]

		start := time.Now()
		var db *mbvmdb.DB
		if _, err := os.Stat(outPath); err == nil {
			db, err = mbvmdb.LoadFromFile(outPath)
			if err != nil {
				db = mbvmdb.New()
			}
		} else {
			db = mbvmdb.New()
		}

		for _, pf := range protoFiles {
			if err := db.IngestProtoDescriptor(pf); err != nil {
				fmt.Fprintf(os.Stderr, "Aviso ao ingerir %s: %v\n", pf, err)
			} else {
				fmt.Printf("✓ Ingerido Protobuf descriptor: %s\n", pf)
			}
		}
		db.CompileGameBytecode()

		if err := db.SaveToFile(outPath); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao salvar banco de proto: %v\n", err)
			os.Exit(1)
		}
		duration := time.Since(start)
		fmt.Printf("⚡ [MBVM-DB] Conhecimento de Protocolo compilado em %v!\n", duration)
		fmt.Printf("  • Arquivo: %s\n", outPath)
		fmt.Printf("  • Símbolos Totais: %d\n", db.TotalSymbols())

	case "slice", "query":
		if len(os.Args) < 4 {
			fmt.Println("Uso: mbvm-db slice <dir_ou_arquivo.mbvmdb> <simbolo_ou_funcao> [profundidade]")
			os.Exit(1)
		}
		target := os.Args[2]
		sym := os.Args[3]
		depth := 2
		if len(os.Args) > 4 {
			_, _ = fmt.Sscanf(os.Args[4], "%d", &depth)
		}

		start := time.Now()
		db, err := loadOrIngest(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao carregar banco: %v\n", err)
			os.Exit(1)
		}

		slice := db.ExtractSlice(sym, depth)
		duration := time.Since(start)

		fmt.Fprintf(os.Stderr, "⚡ [MBVM-DB] Símbolo '%s' fatiado em %v (profundidade %d | %d símbolos | ~%d tokens)\n\n",
			sym, duration, depth, len(slice.Symbols), slice.TokensEstimate)
		fmt.Println(db.ProjectSlice(slice.Symbols))

	case "slice-budget", "budget":
		if len(os.Args) < 5 {
			fmt.Println("Uso: mbvm-db slice-budget <dir_ou_arquivo.mbvmdb> <simbolo> <max_tokens> [profundidade]")
			os.Exit(1)
		}
		target := os.Args[2]
		sym := os.Args[3]
		maxTokens := 500
		_, _ = fmt.Sscanf(os.Args[4], "%d", &maxTokens)
		depth := 3
		if len(os.Args) > 5 {
			_, _ = fmt.Sscanf(os.Args[5], "%d", &depth)
		}

		start := time.Now()
		db, err := loadOrIngest(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao carregar banco: %v\n", err)
			os.Exit(1)
		}

		slice := db.ExtractBudgetSlice(sym, depth, maxTokens)
		duration := time.Since(start)

		fmt.Fprintf(os.Stderr, "⚡ [MBVM-DB] Símbolo '%s' fatiado com teto de %d tokens em %v (%d símbolos | ~%d tokens)\n\n",
			sym, maxTokens, duration, len(slice.Symbols), slice.TokensEstimate)
		fmt.Println(db.ProjectSlice(slice.Symbols))

	case "inspect":
		target := "."
		if len(os.Args) > 2 {
			target = os.Args[2]
		}
		db, err := loadOrIngest(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao inspecionar: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("📦 MBVM-DB Status: %s\n", target)
		fmt.Printf("  • Total de Arquivos: %d\n", db.TotalFiles)
		fmt.Printf("  • Total de Símbolos Indexados: %d\n", db.TotalSymbols())
		fmt.Printf("  • Tamanho do Bytecode Compilado: %d bytes\n", db.BytecodeSize())

	case "verify":
		if len(os.Args) < 3 {
			fmt.Println("Uso: mbvm-db verify <arquivo.mbvmdb>")
			os.Exit(1)
		}
		file := os.Args[2]
		db, err := mbvmdb.LoadFromFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Falha na verificação de integridade: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ [MBVM-DB] Arquivo '%s' íntegro! Magic Header válido, versão 1.0 e CRC32 verificado.\n", file)
		fmt.Printf("  • Total de Símbolos: %d\n", db.TotalSymbols())
		fmt.Printf("  • Bytecode: %d bytes\n", db.BytecodeSize())

	default:
		printUsage()
	}
}

func loadOrIngest(target string) (*mbvmdb.DB, error) {
	if strings.HasSuffix(target, ".mbvmdb") {
		return mbvmdb.LoadFromFile(target)
	}
	db := mbvmdb.New()
	if err := db.IngestGoAST(target); err != nil {
		return nil, err
	}
	return db, nil
}

func printUsage() {
	fmt.Println("MBVM-DB — Micro-Bytecode Virtual Machine Database Engine (Go 1.27)")
	fmt.Println()
	fmt.Println("COMANDOS:")
	fmt.Println("  mbvm-db impact -dir <pacote> -symbol <funcao> [-simulate-add-param int] [-json]  Analisa impacto tipado")
	fmt.Println("  mbvm-db build <dir> [arquivo.mbvmdb]             Compila AST para banco binário indexado persistente")
	fmt.Println("  mbvm-db ingest-proto <saida.mbvmdb> <f.bin ...>  Ingere descritores Protobuf para banco binário de protocolo")
	fmt.Println("  mbvm-db slice <dir|banco.mbvmdb> <simb> [prof]   Extrai fatia subgrafo para IA (-93% tokens)")
	fmt.Println("  mbvm-db slice-budget <dir|banco> <símb> <tokens> Extrai fatia com teto estrito de tokens e anti-ruído")
	fmt.Println("  mbvm-db inspect [dir|banco.mbvmdb]               Inspeciona símbolos e bytecode do projeto")
	fmt.Println("  mbvm-db verify <arquivo.mbvmdb>                  Verifica integridade binária e CRC32")
}
