# MBVM-DB: o que temos e até onde as provas chegam

Auditoria local de **25/10/2026**, em Windows/amd64, Go 1.27.0.
Base Git: `6093a797adaddb34fc418b4d701feef4dd1459f0`, incluindo os arquivos
Go não rastreados presentes no checkout. Não equivale a uma versão publicada.
O [manifesto](evidence/2026-10-25/audit.json), os logs e os hashes delimitam
esta observação. Os relatórios originais dos experimentos foram preservados.

## Definição sustentada pelas evidências

**MBVM-DB é um motor embutido especializado em representar e consultar
grafos de código, com snapshots binários e seleção de contexto. O projeto
também contém análise estática tipada com testemunhos de quebra de compilação,
experimentos de memória associativa HDC e componentes de projeção neural.**

A parte estrutural já executa trabalho útil sobre código real. A parte HDC
tem resultados experimentais delimitados. A presença de código de treinamento
e de vetores não estabelece aprendizado de semântica de programas.

## Critério usado para dizer que algo está provado

| Situação | O que permite afirmar |
|---|---|
| Reproduzido agora | O comportamento ocorreu com os arquivos, entrada e ambiente registrados. |
| Certificado aritmético conferido independentemente | A identidade/classe numérica específica satisfaz as condições verificadas. |
| Resultado histórico salvo | O arquivo registra aquele resultado; sua execução original não foi repetida nesta auditoria. |
| Implementação inspecionada | O mecanismo está implementado; sua qualidade prática ainda exige medição. |
| Não demonstrado ou contradito | Falta evidência suficiente, ou existe um contraexemplo à afirmação. |

Testes finitos não provam correção de todo programa, ausência de todos os bugs,
generalização estatística ou superioridade universal. Hashes vinculam bytes;
não autenticam a autoria nem a execução original de um relatório.

## Inventário das afirmações

| ID | Afirmação defensável | Evidência e status | Limite da conclusão |
|---|---|---|---|
| C01 | O motor compila, indexa fontes e consulta símbolos/dependências. | CLI compilado agora; banco novo com 100 arquivos, 548 símbolos e 13.131 bytes de bytecode. [Log](evidence/2026-10-25/commands.txt). | Não demonstra transações ACID, concorrência irrestrita ou banco de uso geral. |
| C02 | O snapshot observado pode ser salvo/carregado e passa na verificação de formato/CRC32. | Banco novo de 82.997 bytes, verificação executada agora. | CRC não autentica conteúdo; não testamos perda de energia, durabilidade ou recuperação após todo tipo de falha. |
| C03 | Uma mudança controlada de contrato pode produzir quebra real em consumidores estáticos identificados. | `mbvmdb.New` recebeu virtualmente um parâmetro `int`; seis pacotes passaram no build original e falharam no build alterado, com diagnósticos nos pontos de chamada. [Resultado atual](evidence/2026-10-25/module_breakage.json). | Uma alteração em uma configuração de build. Não certifica falhas em runtime, interfaces, reflexão ou consumidores externos não carregados. |
| C04 | Os controles CRT produzem classes numéricas corretas e equivalentes nos dois métodos. | Três executáveis repetidos agora: 12/12 pares em cada variante. [Verificação independente](evidence/2026-10-25/arithmetic_checks.json). | São três variantes relacionadas de 12 fixtures públicas, não 36 problemas independentes inéditos. |
| C05 | Há soluções numéricas concretas para `n=26881` nos registros. | Cinco identidades registradas, correspondentes a duas triplas distintas, conferidas por inteiros exatos nesta auditoria. | Prova dessas instâncias; não prova nova família, conjectura geral ou vantagem de um método/modelo. |
| C06 | HDC preserva contexto estrutural útil no experimento sintético registrado. | 50 sementes, 133 consultas válidas: Recall@10 de 90,65%, contra 23,24% sem contexto e 100% no CSR exato. [Resumo histórico](evidence/2026-10-25/historical_summary.json). | Não repetido agora com `-race`; não mede generalização neural nem descoberta de relações ausentes do grafo. |
| C07 | Um hipervetor binário tem tamanho fixo e recuperação degrada conforme tarefa/ruído. | 8.192 bits = 1.024 bytes de payload; relatório histórico de 120 configurações e 1.440 tentativas de programa por abordagem. | Exclui codebooks, índices, buffers e runtime. Não há capacidade de informação ilimitada nem vantagem demonstrada sobre todo código convencional. |
| C08 | Existem componentes treináveis e projeção 384D para 8.192 bits em Go. | [Treinador](../experiments/hdc-csr/kaggle_train_export_real.py) e [projeção](../experiments/hdc-csr/projection_native.go) inspecionados. | Implementação não prova treino bem-sucedido, melhoria por treinamento ou compreensão de código; a projeção depende do encoder MiniLM externo a esse arquivo. |
| C09 | Há pilotos salvos de uso de um modelo externo com recuperação. | Piloto de 16 solicitações e ablação de oito solicitações sobre um único `n`; resumos recalculados a partir das linhas. | Amostra pequena, falhas de API, tarefas públicas e verificadores textuais heurísticos impedem alegar superioridade científica ou prova formal da prosa. |
| C10 | O slicing reduz a quantidade de contexto retornado em uma consulta. | Consulta atual retornou dois símbolos e estimativa de 52 tokens para orçamento 100. | Seleção de informação não é compressão reversível. Suficiência para a tarefa e tokens reais de um tokenizer não foram certificados. |
| C11 | `slice-budget` usa uma estimativa e pode ultrapassar o orçamento. | Contraexemplo atual: orçamento 1 retornou um símbolo estimado em 32 tokens; [implementação](../pkg/mbvmdb/slice.go) usa bytes/4 e inclui o primeiro candidato mesmo acima do teto. | A antiga afirmação de teto estrito é contradita. |
| C12 | Há um controle simbólico conservador de equivalência em um subconjunto Go. | [Normalizador AST](../experiments/hdc-csr/ast_semantic_baseline.go) inspecionado: tipagem, polinômios inteiros e condicionais restritos; testes existentes não foram repetidos com sucesso nesta auditoria. | Implementação inspecionada, sem certificação formal de todo o algoritmo. Formas diferentes significam equivalência não provada; sintaxe não suportada exige erro/inconclusão. |

Os 548 símbolos do snapshot e os 366 símbolos do grafo tipado não têm o mesmo
universo: o primeiro inclui tipos e outra seleção de arquivos; o segundo
representa funções/métodos dos pacotes selecionados pelo build. Não devem ser
comparados como se fossem medições da mesma cobertura.

## Prova atual mais forte em código real

O comando `impact-module` carregou **26 pacotes**, **366 funções/métodos** e
**723 arestas tipadas**. Para a alteração virtual de `mbvmdb.New`:

- O pacote fornecedor compilou antes e depois da preparação do overlay.
- Apenas as duas chamadas internas do fornecedor foram adaptadas no overlay;
  os consumidores mantiveram suas chamadas originais.
- Foram confirmadas quebras em `cmd/benchmark`, `cmd/compile_all_games`,
  `cmd/gen_sigma_db`, `cmd/mbvm-db`, `cmd/mbvm-mcp` e `experiments/hdc-csr`.
- Os seis builds originais passaram; os seis builds alterados falharam nos
  pontos conhecidos de chamada ao alvo. Nenhum consumidor ficou inconclusivo.
- A implementação conferiu os hashes dos fontes antes/depois e registrou
  `source_files_unmodified=true`. Não houve execução dos aplicativos.

Isso demonstra **impacto de compilação entre pacotes nesse caso concreto**.
Os demais nós alcançáveis permanecem candidatos de impacto. O grafo registra
3.015 chamadas importadas não ligadas; inclui dependências externas ao conjunto
local e não representa cobertura completa de toda execução Go.

O relatório histórico de `StringPool.Intern`, com quatro chamadores e 23 erros,
continua registrado como evidência histórica. O resultado novo acima não
depende de tratá-lo como uma execução da revisão atual.

## Até onde a prova matemática chega

O verificador independente em [summarize.ps1](evidence/2026-10-25/summarize.ps1)
conferiu, sem usar o verificador Go do projeto:

- Positividade e a identidade inteira `4*x*y*z = n*(x*y + x*z + y*z)` para
  cinco registros numéricos da ablação. As duas triplas distintas são
  `(6721, 60222404, 989110288015564)` e `(6721, 65697164, 722668804)`.
- As classes finais dos dois braços em 36 linhas CRT, com resíduos canônicos,
  módulo produto e congruências corretas; nas variantes em duas etapas,
  também conferiu a classe intermediária e sua compatibilidade com a final.
- Por enumeração de todos os resíduos dos pequenos módulos de cada fixture:
  há exatamente uma classe com todas as premissas; retirar qualquer premissa
  deixa mais de uma classe possível no mesmo módulo produto.

Essa indispensabilidade vale para o objetivo específico de classe CRT.
Não significa que duas regras sejam necessárias para resolver Erdős–Straus.
A conferência numérica não autentica chamadas de API, não valida toda a prosa,
não demonstra minimalidade global da prova e não demonstra teorema novo.

## Resultados favoráveis e limites experimentais

O teste HDC multissalto tem **150 consultas candidatas**, das quais **17 sem
relevantes foram excluídas**. O recall de 90,65% é média das 133 restantes,
com classes artificiais e contexto previamente construído. O CSR exato obteve
100% nessa mesma tarefa. Não foi estimado intervalo de confiança nesta auditoria.

No estresse histórico de superposição, com **40% dos bits invertidos**:

| Comprimento do programa | Recuperação integral HDC | Repetição clássica com os mesmos 1.024 bytes |
|---|---:|---:|
| 3 passos | 100% | 100% |
| 8 passos | 97,22% | 100% |
| 32 passos | 5,56% | 58,33% |
| 64 passos | 0% | 2,78% |

Cada linha agrega 36 tentativas, em três sementes. O encoder recebe a sequência
de operações pronta; isso avalia representação/recuperação sob ruído, não
planejamento ou descoberta de regras. A referência clássica é especializada
no alfabeto conhecido de quatro operações. Os controles de composição usam
um executor explícito; seu sucesso não pode ser atribuído exclusivamente ao HDC.

O relatório LSH registra **97,6% de concordância com o Top10 de sua própria
varredura de referência**, mas apenas **1,36% de Recall@10 contra os relevantes
da tarefa**, com média de 774,31 candidatos em 1.000 nós. Concordância com a
referência não equivale a acurácia semântica de 97,6% nem prova de ganho de latência.

No piloto externo, CSR teve dois sucessos marcados em quatro solicitações,
igual ao braço sem memória. Os sucessos textuais dependem em parte de buscas
por termos em [verifier_proofs.go](../experiments/real_eval/verifier_proofs.go).
Na ablação de um único `n`, todas as cinco respostas concluídas forneceram
triplas válidas, em diferentes braços; três solicitações falharam na API.
Não há isolamento causal suficiente para atribuir vantagem à topologia CSR.
No experimento Go de Erdős–Straus, `total_tokens` usa pesos/custos programados,
não consumo medido de uma API. Esse campo não prova economia real de inferência.

## Lacunas de rastreabilidade e validação atual

| Item | Constatação atual | Consequência |
|---|---|---|
| Build e análise estática | CLI compila; `go vet ./...` termina com código 0. | Verificação estática atual aprovada; não substitui testes. |
| Testes exigidos | `go test -race -count=1 -v ./...` bloqueado com CGO=0; ao habilitar CGO apenas no processo, falta `gcc`. | Nenhuma suíte foi declarada aprovada nesta auditoria. |
| Segurança | `govulncheck ./...` reportou nove achados alcançáveis na stdlib do Go 1.27.0; [log completo](evidence/2026-10-25/govulncheck.txt). | Não afirmar auditoria de segurança limpa; achado estático não é exploração demonstrada. |
| Snapshot antigo | `codebase.mbvmdb` passa no CRC, mas tem 69 arquivos/387 símbolos; o snapshot novo tem 100/548 e inclui símbolos ausentes no antigo. | Integridade não prova atualização do índice. O snapshot original foi preservado. |
| Executável antigo | O `mbvm-db.exe` local foi compilado de `fb0c6be...+dirty`; nesta auditoria foi compilado outro executável do checkout atual. | As verificações atuais usam o novo binário temporário. |
| Benchmark dito oficial | O JSON salvo usa `commit_sha=local-manual` e ambiente genérico. | Tratar como registro histórico, sem selo de CI auditado. |
| Workflow HDC | Há trecho de shell incompleto e seção duplicada no arquivo atual. Não foi verificada execução remota bem-sucedida. | A mera presença do workflow não comprova CI aprovado. |
| Artefato neural local | `mbvm_neural_weights.bin` tem 12.662.197 bytes e magic legado `MBVMNN1`, rejeitado pelo auditor do formato atual `MBVM8K1`. | O nome do arquivo não prova conter pesos treinados compatíveis. Não foi feita inferência com esse arquivo. |
| Download Kaggle | `kaggle_output.zip` não abre como ZIP válido nesta cópia. Não foram encontrados checkpoint `.pt`, receipt real-AST ou relatório de sensibilidade treinada, inclusive na busca em arquivos ignorados. | Não foi validado aqui um pipeline neural treinado completo; artefatos em outra máquina não foram auditados. |
| Memória e escala | 1.024 bytes é só payload por hipervetor; a projeção tem 12 MiB de pesos e o núcleo usa IDs `uint16`. | Não afirmar RAM total mínima universal, ausência de vazamentos, capacidade ilimitada ou escala arbitrária. |

## O que seria necessário para ampliar as conclusões

| Afirmação pretendida | Evidência necessária |
|---|---|
| A rede aprendeu semântica de código | Encoder, checkpoint e dados identificados; repositórios de teste separados do treino; pares equivalentes/mutados com testemunhos independentes; mesmo encoder congelado, projeção com/sem treino, baseline MiniLM, busca lexical e AST; resultados por tarefa e incerteza. |
| HDC melhora a recuperação em código real | Mesmos embeddings, consultas, relevantes, orçamento e candidatos com/sem HDC; controle CSR/índice convencional equivalente; medir qualidade, construção, atualização, RAM e latência separadamente. |
| O banco está pronto para produção no cenário escolhido | Ambiente/escala declarados; suíte com `-race -count=1` aprovada; achados de segurança tratados; cargas, concorrência, recuperação e falhas exercitadas. Isso ainda não seria garantia para todo uso. |
| Há ganho matemático ou descoberta | Certificados verificáveis para o objetivo exato, ausência de atalhos, catálogo/compositor iguais nos controles e tarefas novas após congelar o sistema. A prosa do modelo e igualdade de CRT isoladas não bastam. |
| Há economia real de tokens | Tokenizer/modelo declarado, tarefa e correção preservadas, medição real de prompt/completion/raciocínio, falhas separadas e comparação pareada em várias tarefas. |

Essas são condições para novos resultados, não resultados já obtidos.

## Como reproduzir sem sobrescrever os relatórios originais

Na raiz do repositório, com Go e PowerShell disponíveis:

```powershell
$auditTemp = Join-Path ([IO.Path]::GetTempPath()) ('mbvm-proof-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $auditTemp | Out-Null
go build -o "$auditTemp/mbvm-db.exe" ./cmd/mbvm-db
& "$auditTemp/mbvm-db.exe" build . "$auditTemp/current.mbvmdb"
& "$auditTemp/mbvm-db.exe" verify "$auditTemp/current.mbvmdb"
& "$auditTemp/mbvm-db.exe" slice "$auditTemp/current.mbvmdb" ExtractSlice 1
& "$auditTemp/mbvm-db.exe" impact-module -root . -symbol 'github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New' -simulate-add-param int -json -profile
go run ./cmd/run_multilemma_dev -report "$auditTemp/crt_dev_results.json"
go run ./cmd/run_multilemma_depth_dev -report "$auditTemp/crt_depth_dev_results.json"
go run ./cmd/run_multilemma_choice_dev -report "$auditTemp/crt_choice_dev_results.json"
pwsh -NoProfile -File docs/evidence/2026-10-25/summarize.ps1 -FreshCRTDirectory $auditTemp -OutputDirectory "$auditTemp/summary"
go vet ./...
go test -race -count=1 -v ./...
govulncheck ./...
```

O teste com `-race` exige CGO e compilador C compatível; sua falha de ambiente
deve continuar registrada como bloqueio, não ser contornada removendo `-race`.
Consulte os códigos de saída de cada comando: build/CRC, certificados, testes,
segurança e qualidade são verificações distintas.
