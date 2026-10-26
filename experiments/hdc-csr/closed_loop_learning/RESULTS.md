# Primeira execução: 26/10/2026

**O ciclo funcional foi executado. O ganho incremental de Recall@5 não foi
demonstrado: zero das três sementes superou simultaneamente os dois controles.**

O [resumo estruturado](results.json) conserva todos os sete métodos. Os estados,
pesos, observações e resultados por consulta estão em
`output/closed-loop-2026-10-26-pkg-mbvmdb-verified/`, na raiz do repositório.
Esse diretório é local e ignorado pelo Git; o resumo versionável está neste pacote.

## Configuração e escopo

- Go 1.27.0, Windows/amd64; pacote real `pkg/mbvmdb`, com 74 funções/métodos.
- SHA-256 dos fontes selecionados:
  `609c1017e72e4db941a05214321f6ab31b0bdb1d8e66e9febf2a6bac93433541`.
- Em cada semente: sete arquivos de treino, dois de aquisição e quatro de
  avaliação. O catálogo de avaliação tem 74 funções candidatas por consulta.
- Sementes 42/43/44, K=5, 5.000 passos iniciais e 1.000 passos posteriores.
  Os hiperparâmetros do [protocolo](PROTOCOL.md) não foram ajustados aos resultados.
- Metadados HDC e MLP de 113 parâmetros; nenhum checkpoint MiniLM foi carregado.

## Aquisição de conhecimento

| Semente | Propostas verificadas | Relações confirmadas | Consultas de avaliação elegíveis | Consultas sem alvos, excluídas da média |
|---|---:|---:|---:|---:|
| 42 | 95 | 10 | 7 | 16 |
| 43 | 55 | 4 | 6 | 14 |
| 44 | 120 | 10 | 9 | 7 |

Foram **270 verificações**, com **24 confirmações**, correspondentes a **20
relações adquiridas distintas** entre as três execuções. As sementes compartilham
o mesmo pacote; essas contagens não representam 24 descobertas independentes.
As propostas não confirmadas permaneceram observações, sem virar fatos positivos.

Foi conferido nos estados reais: origem dos fatos, exclusão dos símbolos de
avaliação dos exemplos observados, cadeia SHA-256 das três revisões, mudança dos
pesos e orçamento de 6.000 passos no modelo final. O modelo foi atualizado a
partir das evidências relidas do arquivo, após sua persistência.

## Recall@5 nos arquivos reservados

| Método | Semente 42 | Semente 43 | Semente 44 |
|---|---:|---:|---:|
| MLP sem treino | 7,14% | 35,71% | 6,88% |
| HDC fixo, sem treino | 18,71% | 30,16% | 20,11% |
| Regras determinísticas de metadados | 30,27% | 35,32% | 29,10% |
| MLP após treino inicial | 42,18% | 32,54% | 38,36% |
| Mesmo modelo, mais treino sem evidências novas | 49,32% | 32,54% | 38,36% |
| Modelo após atualização com evidências persistidas | 42,18% | 32,54% | 32,80% |
| Referência tipada com fontes completos | 93,54% | 92,46% | 94,97% |

A referência com fontes completos tem informação privilegiada e recupera os
alvos exatos; seu Recall@5 fica abaixo de 100% quando existem mais de cinco
alvos relevantes. Ela não é uma rede com o mesmo orçamento de informação.

Na atualização, duas sementes empataram em Recall@5 com seu próprio modelo
inicial, e uma regrediu. A continuação sem dados novos foi melhor na semente 42
e empatou com a atualização na 43. As métricas secundárias MRR/MAP estão no JSON;
elas não substituem o critério primário depois de observar os resultados.

Isso não prova que o treino incremental seja inútil em geral. Mostra que **este
preditor, com este protocolo e este pacote, não estabeleceu o benefício proposto**.
Há poucos arquivos e consultas elegíveis; os intervalos por arquivo são
descritivos, sem um benchmark externo cego ou teste de superioridade geral.

## Reprodutibilidade e validação

A primeira execução foi preservada em
`output/closed-loop-2026-10-26-pkg-mbvmdb/`. Após reforçar a recusa de fontes
desatualizados no verificador público, o mesmo protocolo foi repetido no
diretório `-verified`. Os hashes dos checkpoints inicial e final foram idênticos
nas três sementes. Nenhum hiperparâmetro foi alterado para favorecer uma métrica.

Os testes específicos passaram com `go test -race -count=1 -v`. Eles incluem
alteração dos rótulos reservados sem mudança nos pesos/fatos, exclusão dos corpos
das representações, métodos homônimos, persistência anterior ao treino,
truncamento/corrupção e rejeição de hipóteses sem evidências. `go vet ./...` passou.

A suíte ampla **passou com `go test -race -count=1 -v ./...`**, numa cópia isolada
com fontes Go conferidos por hash,
para preservar os JSON históricos que alguns testes gravam. Um teste antigo
exigia exatamente quatro chamadores de `StringPool.Intern`; o código atual tem
um quinto, `CompileChessBytecode`. A asserção foi corrigida para manter os quatro
nomes originais como âncoras e exigir igualdade com os chamadores que o compilador
confirma após a mutação. O código de produção desse analisador não foi alterado.

O compilador C portátil LLVM-MinGW 20261020 foi obtido da distribuição oficial,
com SHA-256 `fd5462b7f5d93b2ff981fcd3f9c7e427e47f3203c64ea91c27bcc444eebd74cd`,
para habilitar `-race`, sem alteração do PATH global. Os logs da suíte e da
auditoria estão no diretório local da execução verificada. O `govulncheck`
continua reportando nove achados alcançáveis na biblioteca padrão do Go 1.27.0;
esta alteração não corrige nem declara limpa essa auditoria de segurança.
