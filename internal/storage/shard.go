package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"voltforge/internal/domain"
)

type ShardWriter struct {
	dataDir string
	mu      sync.Mutex
	files   map[string]*os.File
}

func NewShardWriter(dataDir string) *ShardWriter {
	return &ShardWriter{
		dataDir: dataDir,
		files:   make(map[string]*os.File),
	}
}

func (sw *ShardWriter) shardPath(shardID string) string {
	date, protocolID := domain.SplitShardID(shardID)
	dir := filepath.Join(sw.dataDir, "shards", date)
	return filepath.Join(dir, protocolID+".jsonl")
}

func (sw *ShardWriter) Append(shardID string, record any) (int64, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	f, err := sw.getFile(shardID)
	if err != nil {
		return 0, fmt.Errorf("open shard %s: %w", shardID, err)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("marshal record: %w", err)
	}

	pos, err := f.Seek(0, 2)
	if err != nil {
		return 0, fmt.Errorf("seek shard: %w", err)
	}

	line := append(data, '\n')
	if _, err := f.Write(line); err != nil {
		return 0, fmt.Errorf("write shard: %w", err)
	}
	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync shard: %w", err)
	}
	return pos, nil
}

func (sw *ShardWriter) getFile(shardID string) (*os.File, error) {
	if f, ok := sw.files[shardID]; ok {
		return f, nil
	}
	path := sw.shardPath(shardID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	sw.files[shardID] = f
	return f, nil
}

func (sw *ShardWriter) Checksum(shardID string) (string, int, error) {
	path := sw.shardPath(shardID)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("read shard for checksum: %w", err)
	}
	return computeChecksum(data), countLines(data), nil
}

func (sw *ShardWriter) Close() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	var firstErr error
	for _, f := range sw.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	sw.files = make(map[string]*os.File)
	return firstErr
}

func (sw *ShardWriter) Exists(shardID string) bool {
	path := sw.shardPath(shardID)
	_, err := os.Stat(path)
	return err == nil
}

func computeChecksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func countLines(data []byte) int {
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}
	return count
}
