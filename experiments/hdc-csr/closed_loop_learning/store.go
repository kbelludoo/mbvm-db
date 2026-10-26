package closedloop

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"slices"
)

const stateMagic = "MBVMCL1\x00"
const maxStateBytes = 16 << 20

// State files are an experimental sidecar, NOT a new core .mbvmdb version.
// Exclusive creation preserves older revisions; length/CRC reject torn files.
// CRC32 is accidental-corruption detection, not authentication or crash durability.
func writeState(path string, s State) (string, error) {
	if err := s.validate(); err != nil {
		return "", fmt.Errorf("validate state: %w", err)
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("encode state: %w", err)
	}
	if len(payload) > maxStateBytes {
		return "", fmt.Errorf("state exceeds %d byte experiment limit", maxStateBytes)
	}
	raw := make([]byte, 16+len(payload))
	copy(raw, stateMagic)
	binary.LittleEndian.PutUint32(raw[8:12], uint32(len(payload)))
	binary.LittleEndian.PutUint32(raw[12:16], crc32.ChecksumIEEE(payload))
	copy(raw[16:], payload)
	if err := writeNew(path, raw); err != nil {
		return "", err
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}

func readState(path string) (State, string, error) {
	var s State
	f, err := os.Open(path)
	if err != nil {
		return s, "", fmt.Errorf("open state: %w", err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxStateBytes+17))
	if err != nil {
		return s, "", fmt.Errorf("read state: %w", err)
	}
	if len(raw) < 16 || len(raw) > maxStateBytes+16 || string(raw[:8]) != stateMagic {
		return s, "", fmt.Errorf("invalid state header or size")
	}
	payload := raw[16:]
	if int(binary.LittleEndian.Uint32(raw[8:12])) != len(payload) || binary.LittleEndian.Uint32(raw[12:16]) != crc32.ChecksumIEEE(payload) {
		return s, "", fmt.Errorf("state length/CRC mismatch")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return s, "", fmt.Errorf("decode state: %w", err)
	}
	var extra State
	if err := decoder.Decode(&extra); err != io.EOF {
		return s, "", fmt.Errorf("state contains trailing data")
	}
	if err := s.validate(); err != nil {
		return s, "", fmt.Errorf("validate loaded state: %w", err)
	}
	h := sha256.Sum256(raw)
	return s, hex.EncodeToString(h[:]), nil
}

func writeNew(path string, raw []byte) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create immutable artifact: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close artifact: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if _, err = f.Write(raw); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("sync artifact: %w", err)
	}
	return nil
}

func (s State) validate() error {
	if s.Schema != "mbvm-closed-loop-state-v1" || s.Version < 1 || s.Version > 3 {
		return fmt.Errorf("invalid state schema/version")
	}
	if (s.Version < 3 && s.ModelVersion != 1) || (s.Version == 3 && s.ModelVersion != 2) {
		return fmt.Errorf("model version does not match knowledge revision")
	}
	for _, digest := range []string{s.Snapshot.SourceSHA256, s.Snapshot.ContextSHA256} {
		raw, err := hex.DecodeString(digest)
		if err != nil || len(raw) != 32 {
			return fmt.Errorf("invalid snapshot digest")
		}
	}
	if s.Version > 1 {
		raw, err := hex.DecodeString(s.ParentSHA256)
		if err != nil || len(raw) != 32 {
			return fmt.Errorf("later revision needs a parent state hash")
		}
	}
	if err := s.Model.validate(); err != nil {
		return fmt.Errorf("state model: %w", err)
	}
	allowed := append(slices.Clone(s.Split.Train), s.Split.Acquire...)
	seenIDs := map[string]bool{}
	for _, group := range [][]string{s.Split.Train, s.Split.Acquire, s.Split.Test} {
		if len(group) == 0 {
			return fmt.Errorf("empty split group")
		}
		for _, id := range group {
			if id == "" || seenIDs[id] {
				return fmt.Errorf("symbol overlaps split groups")
			}
			seenIDs[id] = true
		}
	}
	observed := map[Relation]Observation{}
	for _, o := range s.Observations {
		if !slices.Contains(allowed, o.Relation.Caller) || !slices.Contains(allowed, o.Relation.Callee) {
			return fmt.Errorf("observation crosses the holdout firewall")
		}
		if o.SourceSHA256 != s.Snapshot.SourceSHA256 || o.ContextSHA256 != s.Snapshot.ContextSHA256 || o.Verifier != "independent-go-types-source-v1" {
			return fmt.Errorf("observation lacks matching source evidence")
		}
		if math.IsNaN(o.Score) || math.IsInf(o.Score, 0) || o.Score < 0 || o.Score > 1 || o.Reason == "" {
			return fmt.Errorf("invalid observation score/reason")
		}
		if o.Confirmed && o.Position == "" {
			return fmt.Errorf("confirmed observation lacks a source position")
		}
		if _, exists := observed[o.Relation]; exists {
			return fmt.Errorf("duplicate observation")
		}
		observed[o.Relation] = o
	}
	seen := map[Relation]bool{}
	for _, fact := range s.Facts {
		o, ok := observed[fact.Evidence.Relation]
		if !ok || !o.Confirmed || o != fact.Evidence || seen[o.Relation] {
			return fmt.Errorf("fact lacks a unique confirmed observation")
		}
		if fact.AddedInVersion < 1 || fact.AddedInVersion > s.Version {
			return fmt.Errorf("fact version exceeds state version")
		}
		seen[o.Relation] = true
	}
	for r, o := range observed {
		if o.Confirmed && !seen[r] {
			return fmt.Errorf("confirmed observation is missing its exact fact")
		}
	}
	return nil
}
