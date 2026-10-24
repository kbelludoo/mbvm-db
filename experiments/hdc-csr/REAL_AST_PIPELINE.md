# MBVM-DB — AST real -> Kaggle -> índice neural 8K (experimental)

Este pipeline usa símbolos **reais** extraídos dos arquivos Go do repositório.
Não modifica `pkg/mbvmdb/`. A exportação binária é **version 2**, header de 64 bytes,
little-endian, 128 palavras `uint64` por símbolo. O resultado da exportação é mantido
fora do Git (`output/`, `*.bin` no `.gitignore`).

## Passo 1 — exportar IDs, código da AST e grafo de chamadas

Na branch `feature/hdc-csr-experimental`, com a versão do Go exigida pelo `go.mod`:

```bash
go test -count=1 ./experiments/hdc-csr
go run ./experiments/hdc-csr/cmd/export_real_ast -root=. -out=output/real_ast_manifest.json
```

O manifesto contém: `symbols[i].id == i`, nome, tipo, pacote, caminho relativo,
declaração Go real, SHA-256 do texto, grafo CSR e relações. O grafo é reconstruído
com a **mesma lógica aproximada de resolução por nomes** de `DB.buildCSR()`. Não
é um call graph com resolução de tipos de compilador. Se a ordem ou o corpus mudar,
exporte **novamente** o manifesto e os vetores; não reutilize índices antigos.

Alternativamente, faça o download do artefato `mbvm-real-go-ast-manifest` do workflow
`.github/workflows/real_ast_kaggle_export.yml` no GitHub Actions.

**Privacidade:** o manifesto contém código Go, inclusive corpos de funções.
Não envie projetos privados a repositórios, Kaggle datasets ou artefatos públicos.

## Passo 2 — no Kaggle, treinar sobre os símbolos Go reais

Envie `real_ast_manifest.json` como entrada para o notebook Kaggle.
Disponibilize o script `experiments/hdc-csr/kaggle_train_export_real.py` no ambiente
com GPU, PyTorch, NumPy, `sentence-transformers` e acesso ao MiniLM.

```bash
python kaggle_train_export_real.py \
  --manifest /kaggle/input/SEU-DATASET/real_ast_manifest.json \
  --out-dir /kaggle/working/real \
  --epochs 5 --batch-size 32 --seed 42
```

O script utiliza **declarações Go reais**, MiniLM (384 floats) e treina uma projeção
`384 -> 8192` por STE para preservar similaridade semântica MiniLM em bits.
Treina/valida em símbolos distintos. **Não treina nas arestas estruturais** e não
declara generalização de código. A tarefa de validação da projeção é apenas preservar
similaridade do professor MiniLM; não substitui avaliação com rótulos externos.

O script grava na pasta `real/`:
- `knowledge_8k_real_ast.bin`: magic `MBVM8K1\\x00`, versão **2**,
  64 bytes de cabeçalho, `N * 1024` bytes de payload, bits little-endian;
- `real_ast_projection.pt`: pesos **efetivamente treinados** no corpus Go;
- `real_export_receipt.json`: hashes SHA-256, digest da origem, contagem e
  tamanho dos vetores, esquema de bits e erro semântico em símbolos reservados.

O arquivo antigo `build_code_graph_brain.py` **não é substituto deste passo**:
ele usa textos sintéticos, escreve `MBVM8K1` **versão 1** e não guarda checkpoints
da projeção. O arquivo local `mbvm_neural_weights.bin` com magic `MBVMNN1` é
outro formato e **não pode ser carregado** como `MBVM8K1`. O índice antigo
`knowledge_8k_supreme.bin` não tem ID -> AST real verificável.

## Passo 3 — verificar o artefato e executar os mesmos quatro motores em Go

Copie os três arquivos produzidos de volta para `output/real/` (não faça commit).

```bash
go run ./experiments/hdc-csr/cmd/evaluate_real_ast \
  -manifest output/real_ast_manifest.json \
  -index output/real/knowledge_8k_real_ast.bin \
  -receipt output/real/real_export_receipt.json \
  -out output/real/real_ast_evaluation.json \
  -queries 100
```

O carregador valida magic, versão, dimensão, comprimento exato do arquivo,
SHA-256 do índice, SHA-256 do manifesto, digest de origem e alinhamento `ID=i`
antes de iniciar o HDC. O relatório compara:

1. `CSR_Local_Semantic`;
2. `Pure_Semantic_Scan`;
3. `HDC_Hybrid_ContextSimilarity`;
4. `HDC_Canonical_VSA_Unbinding`.

A avaliação real de sanidade define como relevantes os destinos do **grafo real**
alcançáveis por `RelCall -> RelCall` em exatamente dois saltos. Essa é uma
**avaliação estrutural derivada do grafo**, não relevância semântica rotulada por
especialistas nem generalização independente. A expressão `QueryRequest.Semantic`
ainda utiliza o vetor do símbolo-fonte; futuras perguntas semânticas precisam de
rótulos humanos ou benchmark de código estabelecido e divisão sem vazamento.

O programa usa limite padrão de 5.000 símbolos, para evitar varreduras e
propagação muito grandes em máquina pequena. Ajuste conscientemente
`-max-symbols` se houver memória e tempo disponíveis.

## Critérios de aceite

- `TestExportRealASTAlignedSymbols` confirma símbolo -> texto -> chamada no
  mesmo ID com AST real;
- `TestLoadRealCorpusRequiresValidReceiptAndIndex` rejeita corrupção, ausência
  de manifesto/recibo e divergência de SHA;
- **Nenhum resultado real é publicado automaticamente no GitHub Pages** até haver
  exportação Kaggle concluída e arquivo de avaliação real conferido;
- O campo `projection_trained` do recibo é uma **declaração do exportador**, não
  uma assinatura criptográfica de terceiro; os hashes detectam divergência de
  arquivos, mas não comprovam a origem histórica do treino;
- O pipeline mantém separado o tempo de construção do contexto e o tempo de
  cada consulta.

Nenhum treinamento Kaggle é afirmado como executado por este commit.
