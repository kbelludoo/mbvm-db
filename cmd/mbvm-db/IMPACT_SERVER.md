# Consultas de impacto em processo persistente

`impact-server` oferece o cache tipado existente da biblioteca pela CLI. Usa
JSONL em stdin/stdout, sem porta de rede e com um único módulo selecionado ao
iniciar. É um protocolo local simples, não um transporte MCP.

```powershell
New-Item -ItemType Directory -Path output -Force | Out-Null
go build -o output/mbvm-db-value.exe ./cmd/mbvm-db

@'
{"id":"first","symbol":"github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New"}
{"id":"followup","symbol":"github.com/kbelludoo/mbvm-db/pkg/mbvmdb.(*StringPool).Intern"}
{"id":"invalidate","op":"invalidate"}
{"id":"reload","symbol":"github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New"}
'@ | ./output/mbvm-db-value.exe impact-server -root .
```

Cada linha gera uma resposta JSON com o `id` recebido. `impact` contém o mesmo
relatório tipado de `impact-module`; `stats` informa `cache_hit`, `go_list_calls`,
`fingerprint_ms`, `impact_ms` e `total_ms`. Um erro de consulta fica em `error`,
sem impedir o processamento das linhas seguintes. EOF encerra o processo.

Operações:

- `{"symbol":"<identidade completa>"}` ou `op="impact"`: consulta de impacto.
- `{"op":"invalidate"}`: remove o grafo do cache; a próxima consulta reconstrói.

O módulo fica fixo no argumento `-root`. Campos desconhecidos, operações
desconhecidas, vários objetos na mesma linha e linhas maiores que 1 MiB são
rejeitados. Não há operações para alterar os fontes ou executar aplicações.

## O que é reutilizado

A primeira consulta carrega o índice. As seguintes fazem o fingerprint novamente
e só reutilizam o grafo se os dados/configuração acompanhados permanecerem iguais.
Os relatórios retornados não expõem os objetos mutáveis internos do cache.

Os [guardas e limites do cache](../../internal/impact/CACHE.md) continuam válidos:
`replace`, workspaces, vendor, symlinks e `go:embed` podem exigir carga fresca.
O cache é conservador nesses casos. Mudanças coordenadas fora dos dados
acompanhados podem exigir `invalidate`; o filesystem não é um snapshot transacional.

Para obter ganho, o cliente mantém **o mesmo processo** aberto durante as
perguntas. Executar um processo novo para cada pergunta continua pagando a carga
inicial. Os comandos existentes `impact`, `impact-module` e `impact-diff`
mantêm seu comportamento de carga fresca.

O benefício medido e a comparação com adjacência convencional estão no
[estudo de valor](../../experiments/value_validation/README.md). Eles não
representam promessa de latência em qualquer máquina, projeto ou configuração.
