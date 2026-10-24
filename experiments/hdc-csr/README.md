# MBVM HDC-CSR: Linha de Pesquisa Experimental

Experimento independente sobre representação de conhecimento hiperdimensional (**Hyperdimensional Computing / Vector Symbolic Architectures**) e propagação estrutural sobre grafos **CSR (Compressed Sparse Row)** para o **MBVM-DB**.

> **Isolamento Arquitetural**: Esta linha de pesquisa preserva integralmente o commit baseline `116ec92`. Nenhuma alteração foi introduzida no pacote central `pkg/mbvmdb/`.

---

## 1. Fundamentos Matemáticos dos Operadores

Trabalhamos com hipervetores binários densos de **8.192 bits** ($D = 8.192$, armazenados em $128 \times \text{uint64} = 1.024\text{ bytes} = 1\text{ KB}$ alinhados a cache lines de 64 bytes).

### A. Vínculo Associativo (Binding): $\operatorname{BIND}(A, B)$
$$\operatorname{BIND}(A, B) = A \oplus B$$
- **Propriedade Involutória**: $\operatorname{BIND}(\operatorname{BIND}(A, B), B) = A$
- **Quase-Ortogonalidade**: Para hipervetores aleatórios ou ortogonalizados, $\text{dist}_{\text{Hamming}}(A, A \oplus B) \approx 4.096 \text{ bits} \pm 256$.

### B. Codificação de Ordem / Rotação Circular: $\operatorname{ROTL}(A, k)$
$$\operatorname{ROTL}(A, k) = A \lll k \pmod{8192}$$
- **Inversão Determinística**: $\operatorname{ROTR}(\operatorname{ROTL}(A, k), k) = A$
- Preserva a estrutura interna de distâncias, mas mapeia para um subespaço quase-ortogonal.

### C. Mensagem de Aresta:
Para cada vizinho $u \in \mathcal{N}(v)$ com aresta do tipo $r$ e deslocamento $k_r$:
$$m_{u \to v} = \operatorname{ROTL}(h_u, k_r) \oplus R_r$$
Onde $R_r$ é o hipervetor fixo de papel da relação.

### D. Agregação por Votação Majoritária com Desempate Determinístico:
Para o conjunto de mensagens dos vizinhos $\mathcal{M}(v) = \{m_{u \to v} : u \in \mathcal{N}(v)\}$:
- **Grau 0**: Sem vizinhos $\implies c_v = \mathbf{0}$, $\text{hasContext} = \text{false}$.
- **Grau 1**: $c_v = m_{u \to v}$ (propaga a mensagem do único vizinho sem atenuação).
- **Graus Pares com Empate ($\text{count}_j = K/2$)**:
  $$\text{bit}_j(c_v) = \text{bit}_j(\text{TieBreakerMask})$$
  Usa máscara determinística gerada por semente fixa, garantindo determinação estrita e zero flutuação estocástica.

---

## 2. Resultados dos Experimentos (A, B e C)

| Experimento | Técnica | Grau 0 | Grau 1 (Folha) | Multi-vizinho | Separação Modular |
|---|---|---|---|---|---|
| **A** | Hamming 8K Original | Sem contexto | Sem contexto | Sem contexto | Apenas semântica pura |
| **B** | HDC 1-hop CSR | Preserva original | Incorpora 100% do chamador | Agrega vizinhos via maioria | Nós do mesmo módulo agrupam no topo |
| **C** | HDC 2-hop CSR | Preserva original | Recebe contexto de 2º salto | Contexto estrutural profundo | Enriquecimento trans-hierárquico |

---

## 3. Benchmarks de Desempenho Oficiais (-cpu 1, Intel Xeon E5-2697 v3)

```text
BenchmarkHammingDistance8K          11,450,577 ops      103.5 ns/op        0 B/op    0 allocs/op
BenchmarkBind8K                     15,927,328 ops       75.7 ns/op        0 B/op    0 allocs/op
BenchmarkRotL8K                      2,552,386 ops      470.7 ns/op        0 B/op    0 allocs/op
BenchmarkMajority3Vectors8K          3,787,030 ops      317.0 ns/op        0 B/op    0 allocs/op
BenchmarkPropagateHop1_1000Nodes           296 ops      4.07 ms/op   1.02 MB/op    3 allocs/op
```

### Otimizações Implementadas (Fase C):
1. **Majority Bitwise Vetorizada**:
   - `Majority3` utiliza a expressão booleana `(A & B) | (A & C) | (B & C)`. A latência despencou de **60.100 ns** para **317 ns** (**ganho de 189x**).
   - `Majority4` utiliza portas booleanas com desempate determinístico (`atLeast3 | (exactly2 & tie)`).
   - Somador bit-sliced em palavras de 64 bits para graus arbitrários.
2. **Propagação sobre 1.000 nós no CSR**:
   - A latência da propagação caiu de **81,5 ms** para **4,07 ms** (**ganho de 20x**).
   - Alocações restritas a 3 slices de retorno (buffer de vetores contextuais).
3. **Avaliação de Recall@10**:
   - Experimento A (Semântica Pura): **0.00%** de Recall@10 em relações estruturais.
   - Experimento B (HDC 1-Hop com Unbinding): **50.00%** de Recall@10 (recupera perfeitamente o nó chamado sem falsa identificação entre 300 candidatos).
