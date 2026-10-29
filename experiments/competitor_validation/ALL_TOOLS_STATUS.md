# Comparação completa — ainda sem vencedor

O critério foi registrado em [ALL_TOOLS_PROTOCOL.md](ALL_TOOLS_PROTOCOL.md):
correção e atualização primeiro, seguida de latência com pesos iguais para
os três projetos. Todos os motores precisam rodar no mesmo ambiente para
uma classificação conjunta. Não combinar resultados Windows e Linux.

| Componente | Estado atual |
|---|---|
| MBVM-DB e gopls | Comparação anterior concluída no Windows, com dados preservados; serão controles no Linux |
| scip-go 0.2.9 | Índices oficiais gerados e validados nos quatro conjuntos; 38/38 respostas equivalentes |
| Glean completo | Aguardando Linux com recursos adequados |
| Sourcegraph completo | Aguardando Linux com recursos adequados |

O índice SCIP não é um servidor Sourcegraph. A validação usa o decodificador
oficial SCIP v0.10.0, com bindings oficiais da mesma versão, e um adaptador que
confere a sintaxe das posições. Esse adaptador não entra numa classificação de
latência dos produtos. Os alvos e os hashes dos fontes coincidem com a rodada
anterior. Há 11 alvos com rótulos independentes no caso controlado; os outros
27 verificam concordância com o grafo tipado, não verdade independente completa.

Após salvar a alteração de `Switch` de `Target` para `Unused`, o snapshot antigo
continuou com duas respostas antigas, como esperado para um índice estático.
Depois de reconstruir o índice oficial, as 11 respostas do caso controlado
voltaram a coincidir com os rótulos atualizados. O índice e JSON anteriores e
posteriores estão separados na rodada v2, para manter os dados originais.

Dados: `output/scip-component-validation-2026-10-26-v2/`. A primeira execução
também foi preservada, mas nela o binário do índice do caso controlado foi
substituído ao reindexar; seu JSON inicial está preservado. Use v2 para auditar
os dois binários. Índices e logs ficam fora dos fontes comparados.

Os testes Go novos passaram com `-race -count=1`; os testes Python verificam
codificações UTF-8/16/32, prioridade dos campos de posição, rejeição de caminhos
fora do módulo e independência das respostas em relação aos rótulos esperados.

Os endpoints acessíveis por SSH apresentaram aproximadamente 1 GB de RAM total,
com cerca de 0,4–0,7 GB disponíveis. Foram feitas apenas inspeções de recursos;
nenhum serviço foi instalado ou teste pesado iniciado nesses servidores.

O Windows local tem memória disponível suficiente, mas não tem Docker nem
Linux operacional. A revisão automática bloqueou a inicialização da VM QEMU,
informando apenas `blocked by policy`. A VM não foi iniciada. A alternativa
solicitada pelo usuário é WSL2: [etapa necessária](WSL_SETUP.md).

Enquanto Glean e Sourcegraph completos não forem executados, a classificação
geral permanece **inconclusiva**. Ausência de ambiente não é derrota técnica
de nenhum concorrente. Este estudo não mede aprendizado neural.
