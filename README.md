# ⚡ MBVM-DB (Micro-Bytecode Virtual Machine Database Engine)

> **High-Performance Machine-Oriented AST & Call-Graph Database Engine in Go 1.27**  
> *Embedded Code Graphs • Binary Snapshots with CRC32 • Context Slicing • Experimental HDC Retrieval*

[![Go Report Card](https://goreportcard.com/badge/github.com/kbelludoo/mbvm-db)](https://goreportcard.com/report/github.com/kbelludoo/mbvm-db)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## Evidências e limites

A [auditoria de evidências de 25/10/2026](docs/EVIDENCIAS_E_LIMITES.md) define quais afirmações são sustentadas pelo checkout atual, quais dependem de relatórios históricos e quais ainda precisam de validação. Ela registra a compilação e consulta do banco, seis pacotes consumidores com quebra de contrato confirmada pelo compilador e controles CRT com aritmética conferida independentemente. Os experimentos HDC medem memória associativa em cenários delimitados; aprendizado de semântica de programas e superioridade geral ainda não foram demonstrados.

## 🏛️ O Que é o MBVM-DB?

O **MBVM-DB** é um motor de banco de dados embutido especializado em grafos de código, AST (Abstract Syntax Tree) e bytecode compacto para alimentar agentes autônomos de IA.

Ao contrário de abordagens estáticas que despejam o código cru (40.000 a 100.000 tokens) ou ASTs inteiras monolíticas (3.000 a 10.000 tokens), o MBVM-DB trata o repositório como um **banco de dados vivo indexado em memória e disco com grafo de chamadas bidirecional em formato CSR**:
1. **Índices, CSR e Opcodes:** Structs, interfaces, métodos e chamadas (`OpCallEdge`) são indexados bidirecionalmente em arrays contíguos CSR (*Compressed Sparse Row*). O custo da travessia depende do subgrafo alcançado.
2. **Sub-graph Slicing Dinâmico:** Você consulta `mbvm-db slice <banco.mbvmdb> <funcao> [profundidade]` e o banco faz uma travessia BFS para selecionar contexto em torno do símbolo.
3. **Slicing por Orçamento Estimado (`slice-budget`):** Prioriza os nós centrais via relevância e penaliza utilitários ruidosos de alto in-degree (ex: loggers). A estimativa usa bytes/4, sem tokenizer específico; o primeiro símbolo é incluído mesmo quando excede o orçamento. Portanto, não há garantia de teto estrito.
4. **Persistência Binária com CRC32:** Salva o banco compilado em arquivo único `.mbvmdb` com cabeçalho mágico (`MBVM\x01\x00`) e verificação de integridade.
5. **Redução de Contexto:** Há medições históricas de projeções menores que o código de entrada. Um slice seleciona parte da informação; a redução não prova compressão sem perdas nem suficiência para qualquer tarefa.

---

## 💡 O Que Aproveitamos de Bancos de Dados Existentes

| Motor / Paradigma | Conceito Absorvido | Aplicação no MBVM-DB |
|---|---|---|
| **Kùzu / Memgraph** *(Graph DBs)* | **CSR & Index-Free Adjacency** | Call-graph bidirecional com busca BFS sobre adjacências CSR. |
| **LMDB / BoltDB** *(Embedded KV)* | **Single-File Binary & Atomic Write** | Formato de arquivo único `.mbvmdb`, arquivo temporário, rename e CRC32. Durabilidade sob falha de energia requer validação específica. |
| **DuckDB / ClickHouse** *(Colunares)* | **String Pool Interning** | String interning com deduplicação; há medições históricas na tabela abaixo. |
| **Meta Glean / Google Kythe** | **Fact-Based Code Graph** | Código como fatos imutáveis e reprodutíveis, eliminando ruído para LLMs. |

---

## 📊 Medições históricas de desempenho

Os números abaixo são registros históricos apresentados pelo projeto. Os logs originais e a revisão exata não estão vinculados nesta tabela; eles não foram reproduzidos na auditoria de 25/10/2026. Não representam garantias universais de latência, memória ou qualidade. Consulte a auditoria antes de citar esses resultados como prova.

```
goos: windows
goarch: amd64
pkg: github.com/kbelludoo/mbvm-db/pkg/mbvmdb
cpu: Intel(R) Xeon(R) CPU E5-2697 v3 @ 2.60GHz

BenchmarkIngestGoSource-28         830        1.44 ms/op      1.61 MB/op      10,031 allocs/op
BenchmarkExtractSliceDepth1-28   53485       22.53 µs/op        8.85 KB/op       474 allocs/op
BenchmarkExtractSliceDepth2-28   50127       24.09 µs/op        9.57 KB/op       485 allocs/op
BenchmarkExtractSliceDepth3-28   48774       24.68 µs/op        9.78 KB/op       493 allocs/op
BenchmarkSaveToFile-28             735        1.62 ms/op       39.18 KB/op     2,427 allocs/op
BenchmarkLoadFromFile-28          4348      269.75 µs/op      132.54 KB/op     3,801 allocs/op
BenchmarkProjectSlice-28        509408        2.24 µs/op        1.42 KB/op        33 allocs/op
BenchmarkStringPoolIntern-28  13124908       91.38 ns/op           0 B/op          0 allocs/op
```

### Reduções de contexto registradas historicamente

As contagens abaixo são registros históricos sem tokenizer e revisão vinculados nesta tabela; não foram recalculadas para o checkout atual.

| Repositório Testado | Código Cru (Tokens) | MB-VM Puro (Tokens) | MBVM-DB Slice (Tokens) | Redução vs Código Cru | Redução Líquida vs MB-VM Puro |
|---|---|---|---|---|---|
| **`huntera`** (21 arqs, 157 KB) | 46.086 tok | 3.219 tok | **224 tok** | **- 99.5%** | **- 93.0%** |
| **`twitch-miner-go`** (7 arqs, 23 KB) | 6.712 tok | 1.100 tok | **355 tok** | **- 94.7%** | **- 67.7%** |
| **`mbvm-db`** (6 arqs, 37 KB) | 10.756 tok | 471 tok | **359 tok** | **- 96.7%** | **- 23.8%** |

---

## 🛡️ Testes automatizados

O MBVM-DB possui testes automatizados cobrindo sintaxe, grafos, persistência e consultas. A auditoria atual não certifica uma contagem de testes aprovados: a execução com `-race` foi bloqueada pela ausência de compilador C no ambiente. Entre as áreas cobertas pelo código dos testes estão:

1. **AST & Sintaxe Go (Bugs 1-25):** Structs vazias, interfaces vazias, closures anônimas, seletores encadeados (`a.B().C()`), generics (`[T any]`), canais, maps de structs, parâmetros variádicos, identificadores em branco (`_`).
2. **Ciclos & Grafos (Bugs 26-45):** Recursão mútua (`A <-> B`), auto-recursão (`A -> A`), dependências diamante, isolamento de subgrafos desconectados, fan-out e fan-in massivos.
3. **Persistência & Armazenamento (Bugs 46-65):** Verificação de Magic Bytes (`MBVM\x01\x00`), detecção de CRC32 corrompido, cabeçalhos truncados, escritas atômicas com rollback em caso de falha.
4. **Condições de Chamada (Bugs 66-85):** Chamadas dentro de `if`, `for`, `defer`, `go routine`, `return`, `switch`, `select`, métodos chamados em variáveis e ponteiros duplos (`**T`).
5. **Integração Real (Bugs 86-102 + Storage/Slice):** Tipos da biblioteca padrão (`time.Time`, `context.Context`), ordenação 100% determinística de pacotes/símbolos, ausência de vazamento de memória em 500+ queries simultâneas.

Para rodar toda a suíte de testes:
```bash
go test -race -count=1 -v ./pkg/mbvmdb
```

---

## 💻 Como Usar o CLI

### 1. Compilar o Banco Binário Persistente
```bash
mbvm-db build . codebase.mbvmdb
```

### 2. Verificar a Integridade e o CRC32 do Banco
```bash
mbvm-db verify codebase.mbvmdb
```

### 3. Extrair Fatia Específica (Subgrafo com Profundidade)
```bash
# Consulta direto do banco binário em microssegundos
mbvm-db slice codebase.mbvmdb "ExtractSlice" 1
```

### 4. Extrair Fatia com Teto Estrito de Tokens (Anti-Ruído / Context-Budget)
```bash
# Usa orçamento estimado de 100 tokens; o primeiro símbolo pode excedê-lo
mbvm-db slice-budget codebase.mbvmdb "ExtractSlice" 100
```

### 5. Inspecionar Estatísticas do Banco
```bash
mbvm-db inspect codebase.mbvmdb
```

### 6. Rodar Benchmark Comparativo
```bash
go run ./cmd/benchmark . ExtractSlice
```

---

## 📄 Licença
Distribuído sob a licença MIT.
