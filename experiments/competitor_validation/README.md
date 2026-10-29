# MBVM-DB versus gopls — resultado de 26/10/2026

Foi executada uma comparação real do `impact-server` existente com o
**gopls v0.24.0 por LSP**, mantendo ambos os processos abertos. O resultado
mostra vantagem específica do MBVM em duas cargas, uma desvantagem importante
na terceira e sensibilidade do fingerprint ao conteúdo do workspace.

## Medição com fontes equivalentes em cópias limpas

Cada célula contém as medianas de três execuções completas, em milissegundos.
Menor é melhor. Os caches e relatórios ficaram fora dos módulos comparados.

| Projeto | MBVM: execuções 1 / 2 / 3 | gopls: execuções 1 / 2 / 3 | Resultado observado |
|---|---:|---:|---|
| MBVM-DB | 29,08 / 28,37 / 27,81 | 60,05 / 63,54 / 61,20 | MBVM aproximadamente 2,2× mais rápido |
| twitch-miner-go | 628,30 / 606,19 / 616,81 | 41,40 / 41,22 / 43,45 | gopls aproximadamente 15× mais rápido |
| kick-miner-go | 7,14 / 7,11 / 7,14 | 49,94 / 48,99 / 49,46 | MBVM aproximadamente 6,9× mais rápido |

As razões aproximadas usam a mediana das três medianas de cada ferramenta.
Não são razões da melhor amostra selecionada. O p95 por execução ficou em
30,03–33,42 ms contra 115,63–127,55 ms no MBVM-DB; 621,33–649,59 ms contra
92,53–103,19 ms no Twitch; 8,00–8,17 ms contra 94,10–109,76 ms no Kick,
sempre na ordem MBVM / gopls. Tempos de inicialização e preparação constam
separadamente nos JSONs; esta tabela contém consultas seguintes.

O cache do MBVM teve 84/84 acertos por execução no próprio projeto e 56/56
no Kick. No Twitch, teve **0/49**: o guarda atual recusa reaproveitamento quando
há `go:embed`, pois não acompanha alterações dos assets incorporados. O custo
da reconstrução fica visível no resultado; não foi removido da comparação.

## Correção e alcance da evidência

- 38 perguntas distintas por execução: 11 no caso controlado, 12 no MBVM-DB,
  sete no Twitch e oito no Kick. Sete repetições de cada pergunta.
- Seis execuções formais: **1.596 comparações com conjuntos iguais**, sendo
  798 no workspace original e 798 em cópias limpas. Repetições não equivalem
  a 1.596 programas ou problemas independentes.
- O caso controlado tem respostas literais independentes dos dois motores:
  **462/462 consultas corretas**. Cobre chamadas entre pacotes, métodos como
  chamadores, closures, go/defer, genéricos, referências como valores, recursão
  e referências exclusivas de testes.
- As 1.134 consultas restantes verificam concordância com o grafo tipado
  existente do MBVM. Não constituem uma verdade independente de cobertura
  completa de Go. A normalização compartilha o inventário de identidades,
  mas não filtra respostas pelas arestas ou pelos conjuntos esperados.
- Em cada execução foi salva a mesma mudança de uma chamada de `Target`
  para `Unused` no caso temporário. Os dois sistemas reconheceram os conjuntos
  atualizados: 12 verificações de consulta após seis alterações equivalentes.
- Hashes dos fontes foram iguais entre os dois cenários e antes/depois das
  consultas. As alterações intencionais atingiram somente o caso temporário.

O recorte é: **chamadores diretos distintos, excluindo o próprio alvo, de
funções de pacote**, entre funções nomeadas de fontes de produção do módulo.
Métodos concretos e de interfaces não são alvos desta comparação. Inicialização
global, outras configurações de build, dispatch dinâmico, reflexão e todo o
conjunto de funcionalidades dos produtos exigem avaliações próprias.

O gopls fornece hierarquia de chamadas usando referências. Seu adaptador
confere a sintaxe da posição recebida e descarta referências fora da expressão
de uma chamada, arquivos de teste, inicializadores globais e autorreferências.
Mensagens originais e quantidade de posições descartadas foram preservadas.
O custo dessa normalização entra no tempo medido; o tempo RPC também é salvo.
O MBVM calcula ainda candidatos transitivos em sua resposta normal. Portanto,
avaliamos APIs existentes, não algoritmos isolados realizando trabalho idêntico.

## Workspace original: medições também preservadas

| Projeto | MBVM: execuções 1 / 2 / 3 | gopls: execuções 1 / 2 / 3 |
|---|---:|---:|
| MBVM-DB | 110,54 / 132,48 / 156,86 | 102,29 / 107,82 / 59,08 |
| twitch-miner-go | 726,79 / 690,25 / 689,85 | 48,43 / 79,91 / 41,28 |
| kick-miner-go | 41,13 / 39,27 / 39,34 | 59,48 / 45,37 / 45,71 |

Nesta preparação, o cache do gopls foi colocado em `output/` dentro do checkout
do MBVM. O fingerprint do MBVM percorre essa árvore, apesar de ignorada pelo
Git, incluindo os diretórios criados pelo próprio teste. Isso introduz trabalho
adicional. As cópias limpas evitam esse efeito, sem alterar motores, binários,
fontes ou perguntas. A diferença entre cenários não isola causalmente quanto
tempo cada diretório acrescenta. É também um limite prático do fingerprint atual.

O caso pequeno teve ordenação de velocidade variável entre lançamentos;
não é apresentado como vitória estável de qualquer ferramenta. A execução
piloto somente do caso pequeno também foi preservada e não entra nos totais.

## Reprodução e arquivos

Plano inicial: [PROTOCOL.md](PROTOCOL.md). A decisão de repetir o teste está em
[REPLICATION.md](REPLICATION.md); a correção do ambiente, definida antes das
novas medições, está em [CONTROLLED_WORKSPACE.md](CONTROLLED_WORKSPACE.md).

No workspace do projeto:

```powershell
$env:GOBIN = "$PWD/output/competitor-tools"
go install golang.org/x/tools/gopls@v0.24.0
$env:CGO_ENABLED = '0'
go build -o output/competitor-tools/mbvm-db.exe ./cmd/mbvm-db
go build -o output/competitor-tools/compare.exe ./experiments/competitor_validation/cmd/compare

$env:GOMAXPROCS = '1'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOFLAGS = '-mod=readonly'
# Escolha uma pasta de cache fora dos módulos comparados.
$env:GOPLSCACHE = "$env:TEMP/gopls-comparison-new-cache"
./output/competitor-tools/compare.exe -out output/new-comparison `
  -roots 'C:/copias/mbvm-db;C:/copias/twitch-miner-go;C:/copias/kick-miner-go'
```

Para reproduzir o cenário limpo, copie os arquivos rastreados e não ignorados
de cada checkout, preservando alterações locais e assets. Inclua também os
fontes Go e arquivos de módulo ignorados, caso existam, fora de diretórios de
saída. Confira os hashes. Deixe caches, logs e saída fora das cópias; use uma
pasta de saída nova para cada lançamento. Nunca execute os aplicativos analisados.

Dados brutos locais:

- `output/gopls-comparison-2026-10-26{,-repeat-2,-repeat-3}/report.json`;
- `output/gopls-comparison-clean-2026-10-26-{1,2,3}/report.json`;
- cada pasta `case-N/` contém mensagens LSP/JSONL originais e stderr;
- `output/competitor-tools/` contém binários, informações de build, CPU,
  mapeamento das cópias, equivalência dos fontes e logs de validação;
- [validation.json](validation.json) e [verify.ps1](verify.ps1) auditam
  respostas, rótulos independentes, hashes, contagens, medianas e p95.

Ambiente: Windows amd64, Go 1.27.0, GOMAXPROCS=1, Xeon E5-2697 v3.
Caches do compilador e do sistema operacional podem estar aquecidos. O servidor
gopls mantém seus recursos normais de análise; não exigimos repouso de todas as
atividades internas. As repetições limitam, mas não eliminam, variação de carga.
Não medimos RAM total da árvore de processos nem tamanho comparável de índices.

A suíte completa `go test -race -count=1 -v ./...` e `go vet ./...` passaram
em cópia isolada com os mesmos 201 fontes Go/arquivos de módulo. Os testes novos
verificam os rótulos, filtragem de referências, framing LSP, UTF-16, URIs e drift
de fonte/configuração. O isolamento preserva relatórios históricos que testes
anteriores podem regravar. Não houve mudança no motor de produção.

O resultado demonstra utilidade competitiva **neste recorte de consultas
estruturais**. Não mede aprendizado neural e não é comparação executada contra
Glean ou uma implantação Sourcegraph. Referências oficiais:
[gopls](https://go.dev/gopls/),
[interface CLI experimental](https://go.dev/gopls/command-line),
[semântica de referências](https://go.dev/gopls/features/navigation).
