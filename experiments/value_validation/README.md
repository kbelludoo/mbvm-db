# Valor demonstrado: consultas de impacto tipado em Go

**Há valor prático mensurável na reutilização verificada do índice para
consultas repetidas de impacto. Este estudo não demonstrou vantagem geral
do CSR sobre um índice convencional nem ganho da rede neural.**

O [protocolo](PROTOCOL.md) foi definido antes da primeira medição. O
[resumo estruturado](results.json) conserva os três projetos e o caso sem ganho.
Os relatórios completos estão em `output/value-validation-2026-10-26-v2/`.

## Resultado em três projetos reais

Go 1.27.0, Windows/amd64, GOMAXPROCS=1; sete pares de medições, com a ordem
alternada e os exports do compilador já aquecidos nos dois caminhos:

| Projeto | Símbolos comparados | Análise fresca + consulta, mediana | Consulta com cache e fingerprint, mediana | Razão entre os tempos |
|---|---:|---:|---:|---:|
| MBVM-DB | 413 | 1.307,14 ms | 71,19 ms | 18,36× |
| twitch-miner-go | 28 | 679,21 ms | 697,18 ms | 0,97× — sem ganho |
| kick-miner-go | 55 | 679,91 ms | 38,43 ms | 17,69× |

O ganho compara **recarregar a análise tipada a cada pergunta** com **reutilizar
o grafo no mesmo processo, conferindo os fontes/configuração novamente**.
Não compara o MBVM-DB com todos os produtos do mercado. Ferramentas convencionais
também podem manter índices e caches; isso não é uma exclusividade técnica.

A inicialização do cache custou, respectivamente, 1.462,02 ms, 677,37 ms e
765,06 ms, além dos custos de compilação/export já aquecidos. O ganho de 18×
refere-se às consultas seguintes, não à primeira chamada ou a todo fluxo possível.
No twitch-miner-go, o mecanismo detectou `go:embed` e recorreu à análise fresca,
pois o hash atual não acompanha os assets embutidos. Esse caso foi mantido.

## Correção observada e contexto entregue

- **496/496 respostas** de impacto concordaram com uma implementação separada
  de BFS sobre adjacências indexadas, incluindo nós sem referências e ciclos.
- **Nove alterações controladas de assinatura**, três por projeto, produziram
  os erros esperados em **16 pares alvo/chamador** identificados estaticamente.
  Os hashes dos fontes antes/depois permaneceram iguais.
- Uma checagem adicional com o **`go build` real**, nos construtores públicos dos
  três projetos, confirmou quebra em **oito pacotes consumidores**: os builds
  originais passaram, o fornecedor virtual adaptado compilou, e os consumidores
  falharam nos pontos de chamada conhecidos. Nenhum ficou inconclusivo.
- A mediana dos relatórios selecionados foi de 561 bytes no MBVM-DB, 603 bytes
  no twitch-miner-go e 642,5 bytes no kick-miner-go, contra inventários tipados
  completos de 264.383, 13.307 e 31.254 bytes, respectivamente.

Isso mede a resposta à pergunta específica de impacto. O índice convencional
retorna o mesmo contexto; a economia vem de consultar em vez de despejar todo o
grafo. Não é compressão reversível, contagem exata de tokens ou garantia de que
um agente resolva qualquer tarefa com esse contexto.

A equivalência sobre os mesmos fatos tipados não prova cobertura completa do
runtime Go. As testemunhas do compilador são casos finitos, com parâmetro `int`
adicional, e não falhas reais de execução ou prova formal do analisador.

## Comparação com um índice convencional competente

O controle usa `map[uint32][]uint32`, busca O(1) do símbolo, BFS independente,
arrays novos de trabalho e a mesma construção/ordenação do resultado. Os fatos
de entrada são os mesmos; o controle não reutiliza os offsets CSR.

| Projeto | CSR atual por consulta pré-indexada | Adjacência indexada por consulta |
|---|---:|---:|
| MBVM-DB | 3,58 µs | 2,84 µs |
| twitch-miner-go | 0,57 µs | 0,58 µs |
| kick-miner-go | 1,11 µs | 1,04 µs |

Essas medições **não sustentam superioridade geral do CSR**. O valor operacional
medido está em preservar a análise tipada entre consultas, com conferência de
atualização dos fontes e uma resposta correta e utilizável. Não se deve chamar
a travessia de poucos microssegundos de tempo total: a leitura/hash dos fontes
continua custando dezenas de milissegundos nos casos com cache.

As primeiras micro-medições com lotes de 1.000 consultas tiveram resolução
insuficiente em projetos pequenos e foram preservadas em
`output/value-validation-2026-10-26/`. A emenda v2 aumentou igualmente os dois
métodos para 100.000 consultas por lote, mantendo cinco lotes e ordem alternada.
Os cenários, algoritmos e critério de conclusão foram mantidos.

## Recurso utilizável entregue

A CLI agora oferece [impact-server](../../cmd/mbvm-db/IMPACT_SERVER.md), um
processo local que recebe perguntas JSONL por stdin e devolve respostas e tempos
por stdout. Ele reaproveita o índice e mantém os guardas de invalidação existentes.

Na demonstração com o executável compilado, três perguntas no mesmo processo
levaram aproximadamente 1.182 ms, 59 ms e 59 ms. Só a primeira fez `go list`;
as duas seguintes foram hits. A segunda pergunta consultou outro símbolo, e não
apenas a resposta da primeira. Veja `server-demo.jsonl` no diretório da execução.

## Reproduzir

Na raiz do projeto, com os módulos disponíveis localmente:

```powershell
$env:GOMAXPROCS = '1'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
go run ./experiments/value_validation/cmd/validate-value `
  -roots '.;../twitch-miner-go;../kick-miner-go' -out output/value-new-run
```

`-roots '.'` permite avaliar somente este checkout. Projetos ausentes, falhas de
carregamento e cache desativado ficam registrados; não são substituídos por casos
favoráveis. O diretório de saída deve ser novo ou vazio. As aplicações analisadas
não são executadas e seus fontes não são modificados.

```powershell
go test -race -count=1 -v ./experiments/value_validation/... ./cmd/mbvm-db
go vet ./...
```

A suíte completa `go test -race -count=1 -v ./...` passou numa cópia isolada,
para não sobrescrever relatórios históricos produzidos pelos testes. Os fontes
Go dessa cópia são conferidos por hash contra o checkout. `go vet ./...` passou.
O `govulncheck` continua reportando nove achados na stdlib do Go 1.27.0; esta
validação não declara segurança de produção ou ausência de vulnerabilidades.

## Limite e consequência prática

Este estudo sustenta um uso concreto: **um agente ou aplicação pode manter um
processo de análise de impacto Go, receber respostas tipadas menores e evitar
reconstruir o índice em cada pergunta quando os guardas permitem reutilização**.

Não sustenta superioridade sobre Glean/gopls/outros produtos completos, novidade
algorítmica ou benefício neural. O resultado anterior de zero ganho incremental
em três sementes continua documentado em
[closed_loop_learning](../hdc-csr/closed_loop_learning/RESULTS.md).
