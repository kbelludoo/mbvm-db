# Aprendizado em ciclo fechado: experimento de dependências Go

Implementa **propor → verificar → persistir → continuar o treino → avaliar**
em um único pacote Go, com comandos e artefatos locais. O ciclo é experimental;
não altera `pkg/mbvmdb`, não inicia treinamento externo e não chama modelos pagos.

O [protocolo](PROTOCOL.md) fixa a primeira execução e suas condições. Uma
execução sem ganho ou com regressão termina normalmente e conserva o resultado.

## Executar

Na raiz do repositório:

```powershell
go run ./experiments/hdc-csr/closed_loop_learning/cmd/closed-loop `
  -dir ./pkg/mbvmdb -out output/closed-loop-new-run
```

O diretório de saída precisa ser novo ou vazio. Por padrão: sementes 42, 43 e
44; TopK=5; 5.000 passos iniciais e 1.000 passos incrementais. `-seeds`,
`-top-k`, `-train-steps` e `-update-steps` permitem outros protocolos, que devem
ser identificados como experimentos novos, sem selecionar resultados favoráveis.

## Como o ciclo funciona

1. O índice tipado existente fornece funções/métodos e relações estáticas locais.
2. Os arquivos são separados em treino, aquisição e avaliação final. Um símbolo
   de avaliação não participa de exemplos de treino em nenhum dos dois papéis.
3. O modelo inicial aprende com pares do subconjunto de treino. Suas propostas
   para os arquivos de aquisição são apenas hipóteses com pontuações.
4. Outro extrator recarrega e tipa os fontes originais. Ele confirma identidade
   de pacote, receiver e função, registra posições e rejeita fontes desatualizados.
5. Somente relações confirmadas entram em `Facts`. Propostas não observadas como
   chamadas estáticas locais ficam em `Observations`, sem promoção a fatos.
6. O programa persiste e **relê** a revisão com evidências antes de usar os novos
   exemplos. Continua o modelo existente, com replay dos exemplos iniciais.
7. Avalia funções de arquivos reservados, com todas as funções do pacote como
   candidatas. O mesmo conjunto e o mesmo K são usados por todas as referências.

O preditor usa nomes, receivers, assinaturas e informação de mesmo arquivo.
Os corpos de funções, expressões de chamada e relações ocultadas não entram
nas características. HDC8192 é uma codificação determinística desses metadados;
o MLP tem **113 parâmetros treináveis**. Não é a projeção MiniLM do outro
experimento, não usa checkpoint fictício e não demonstra aprendizado desse encoder.

## Controles e resultados

O relatório compara: MLP sem treino, HDC sem treino, regras de metadados,
modelo treinado inicial, continuação sem evidências novas, continuação com
evidências persistidas e extração tipada com acesso aos fontes completos.

A continuação sem fatos novos recebe o mesmo orçamento de passos. Assim,
uma melhora por mais otimização não é automaticamente atribuída ao ciclo.
A referência com fontes completos tem acesso privilegiado e é identificada
separadamente. O analisador já consegue resolver essas relações quando recebe
o código; o experimento não prova que uma rede seja necessária para essa tarefa.

`report.json` registra Recall@K, Precision@K, MRR, MAP, contagens de verificação,
partições, hashes dos estados e diferenças pareadas. Os intervalos de bootstrap
por arquivo são descritivos em uma amostra pública pequena. A soma de sementes
não representa repositórios novos e independentes. Consulte também
[o registro da primeira execução](RESULTS.md).

## Artefatos

Para cada semente:

- `state-0001.mbvmcl`: fatos iniciais confirmados e modelo versão 1.
- `verified-before-training.mbvmcl`: revisão 2, evidências adquiridas e o mesmo
  modelo versão 1, registrada antes do treino incremental.
- `state-0003.mbvmcl`: revisão 3, fatos/evidências e modelo atualizado versão 2.
- `evaluation.jsonl`: métricas por consulta e por método.

Os campos `version_one_sha256` e `version_two_sha256` do relatório identificam
os checkpoints dos **modelos** 1 e 2; o segundo está na revisão de conhecimento 3.
Cada revisão posterior aponta para o SHA-256 da revisão anterior.

`.mbvmcl` é um sidecar binário experimental, com metadados e pesos serializados,
comprimento e CRC32. Os fatos ficam separados dos pesos e das hipóteses.
Esse formato não substitui `.mbvmdb` e não promete autenticação ou transações.

## Validação

```powershell
go test -race -count=1 -v ./experiments/hdc-csr/closed_loop_learning/...
go vet ./experiments/hdc-csr/closed_loop_learning/...
```

Os testes verificam o ciclo completo, a cadeia de persistência, receptores de
métodos com mesmo nome, fontes/contexto alterados, corrupção/truncamento,
proteção contra sobrescrita e hipóteses sem evidência. Um teste altera os rótulos
reservados e exige os mesmos hashes de modelos/fatos; outro muda chamadas nos
corpos e exige representações idênticas. Nenhum teste exige ganho em dados reais.

No Windows, `-race` exige CGO e um compilador C compatível. A execução local de
desenvolvimento usou LLVM-MinGW portátil, sem alterar o PATH global. O workflow
específico em `.github/workflows/closed_loop_learning.yml` mantém esses testes
e registra resultados adversos; sua criação não significa que já executou no CI.
