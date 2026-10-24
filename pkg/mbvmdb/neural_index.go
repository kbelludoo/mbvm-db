package mbvmdb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"os"
)

// Constantes do formato binário neural 8K alinhado a cache line (64 bytes)
const (
	NeuralMagic8K      = "MBVM8K1\x00" // 8 bytes
	NeuralHeaderSize   = 64            // Alinhado a 64 bytes (Cache Line L1/L2)
	NeuralVectorBits   = 8192          // 8.192 bits
	NeuralWordsPerVec  = 128           // 128 x uint64 = 8.192 bits = 1.024 bytes
	NeuralVectorBytes  = 1024          // 1 KB exato por símbolo
)

// NeuralIndex armazena o índice semântico binário unificado com a MBVM.
// Os dados podem ser lidos diretamente de arquivo via streaming ou mantidos em slice alinhado de uint64.
type NeuralIndex struct {
	Version      uint32
	TotalSymbols uint32
	Words        []uint64 // TotalSymbols * 128 uint64 (acesso direto sem cópia)
}

// NewNeuralIndex cria um novo índice vazio em memória.
func NewNeuralIndex(totalSymbols uint32) *NeuralIndex {
	return &NeuralIndex{
		Version:      1,
		TotalSymbols: totalSymbols,
		Words:        make([]uint64, int(totalSymbols)*NeuralWordsPerVec),
	}
}

// SetVector copia 128 palavras uint64 para o símbolo indicado.
func (idx *NeuralIndex) SetVector(symbolID uint32, vec []uint64) error {
	if symbolID >= idx.TotalSymbols {
		return fmt.Errorf("symbolID %d fora dos limites (total: %d)", symbolID, idx.TotalSymbols)
	}
	if len(vec) != NeuralWordsPerVec {
		return fmt.Errorf("vetor deve ter exatamente %d uint64s, recebido %d", NeuralWordsPerVec, len(vec))
	}
	offset := int(symbolID) * NeuralWordsPerVec
	copy(idx.Words[offset:offset+NeuralWordsPerVec], vec)
	return nil
}

// GetVector retorna o slice das 128 palavras uint64 do símbolo sem alocação adicional.
func (idx *NeuralIndex) GetVector(symbolID uint32) []uint64 {
	if symbolID >= idx.TotalSymbols {
		return nil
	}
	offset := int(symbolID) * NeuralWordsPerVec
	return idx.Words[offset : offset+NeuralWordsPerVec]
}

// HammingDistance calcula a distância de Hamming entre dois símbolos em nanossegundos
// usando 128 operações XOR e contagem de bits por hardware (POPCNT).
func (idx *NeuralIndex) HammingDistance(idA, idB uint32) int {
	offsetA := int(idA) * NeuralWordsPerVec
	offsetB := int(idB) * NeuralWordsPerVec
	words := idx.Words

	var dist int
	// Desenrolamento do loop para máxima vazão de instruções na CPU
	for i := 0; i < NeuralWordsPerVec; i++ {
		dist += bits.OnesCount64(words[offsetA+i] ^ words[offsetB+i])
	}
	return dist
}

// SaveToFile grava o índice neural no disco respeitando o alinhamento de 64 bytes.
func (idx *NeuralIndex) SaveToFile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, NeuralHeaderSize)
	copy(header[0:8], []byte(NeuralMagic8K))
	binary.LittleEndian.PutUint32(header[8:12], idx.Version)
	binary.LittleEndian.PutUint32(header[12:16], idx.TotalSymbols)
	binary.LittleEndian.PutUint16(header[16:18], uint16(NeuralVectorBits))
	// header[18:64] são preenchidos com zeros (padding para 64 bytes)

	if _, err := f.Write(header); err != nil {
		return err
	}

	// Escreve as palavras uint64 diretamente em little-endian
	buf := make([]byte, 8)
	for _, w := range idx.Words {
		binary.LittleEndian.PutUint64(buf, w)
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

// LoadFromFile carrega o índice do disco validando a assinatura e o alinhamento de 64 bytes.
func LoadNeuralIndexFromFile(filename string) (*NeuralIndex, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	header := make([]byte, NeuralHeaderSize)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, fmt.Errorf("falha ao ler header de 64 bytes: %w", err)
	}

	if string(header[0:8]) != NeuralMagic8K {
		return nil, errors.New("cabeçalho neural inválido: esperado MBVM8K1\\x00")
	}

	version := binary.LittleEndian.Uint32(header[8:12])
	totalSymbols := binary.LittleEndian.Uint32(header[12:16])
	dim := binary.LittleEndian.Uint16(header[16:18])

	_ = version
	if dim != NeuralVectorBits {
		return nil, fmt.Errorf("dimensão incompatível: %d (esperado %d)", dim, NeuralVectorBits)
	}

	idx := NewNeuralIndex(totalSymbols)
	buf := make([]byte, 8)
	totalWords := int(totalSymbols) * NeuralWordsPerVec
	for i := 0; i < totalWords; i++ {
		if _, err := io.ReadFull(f, buf); err != nil {
			return nil, fmt.Errorf("falha ao ler vetor no índice %d: %w", i, err)
		}
		idx.Words[i] = binary.LittleEndian.Uint64(buf)
	}

	return idx, nil
}
