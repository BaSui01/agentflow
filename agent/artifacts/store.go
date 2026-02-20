package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore使用本地文件系统执行ArtifactStore.
type FileStore struct {
	basePath string
	mu       sync.RWMutex
	index    map[string]*Artifact
}

// NewFileStore创建了一个新的基于文件的文物商店.
func NewFileStore(basePath string) (*FileStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path: %w", err)
	}

	store := &FileStore{
		basePath: basePath,
		index:    make(map[string]*Artifact),
	}

	if err := store.loadIndex(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *FileStore) Save(ctx context.Context, artifact *Artifact, data io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 读取所有数据以计算校验和大小
	buf := new(bytes.Buffer)
	size, err := io.Copy(buf, data)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	dataBytes := buf.Bytes()
	hash := sha256.Sum256(dataBytes)
	artifact.Checksum = hex.EncodeToString(hash[:])
	artifact.Size = size

	// 创建存储路径
	artifactDir := filepath.Join(s.basePath, artifact.ID)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return fmt.Errorf("failed to create artifact dir: %w", err)
	}

	// 写入数据文件
	dataPath := filepath.Join(artifactDir, "data")
	if err := os.WriteFile(dataPath, dataBytes, 0644); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	artifact.StoragePath = dataPath

	// 写入元数据
	metaPath := filepath.Join(artifactDir, "metadata.json")
	metaData, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	s.index[artifact.ID] = artifact
	return s.saveIndex()
}

func (s *FileStore) Load(ctx context.Context, artifactID string) (*Artifact, io.ReadCloser, error) {
	s.mu.RLock()
	artifact, ok := s.index[artifactID]
	s.mu.RUnlock()

	if !ok {
		return nil, nil, fmt.Errorf("artifact not found: %s", artifactID)
	}

	file, err := os.Open(artifact.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open data: %w", err)
	}

	return artifact, file, nil
}

func (s *FileStore) GetMetadata(ctx context.Context, artifactID string) (*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	artifact, ok := s.index[artifactID]
	if !ok {
		return nil, fmt.Errorf("artifact not found: %s", artifactID)
	}
	return artifact, nil
}

func (s *FileStore) Delete(ctx context.Context, artifactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	artifact, ok := s.index[artifactID]
	if !ok {
		return fmt.Errorf("artifact not found: %s", artifactID)
	}

	artifactDir := filepath.Dir(artifact.StoragePath)
	if err := os.RemoveAll(artifactDir); err != nil {
		return fmt.Errorf("failed to delete artifact: %w", err)
	}

	delete(s.index, artifactID)
	return s.saveIndex()
}

func (s *FileStore) List(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Artifact
	for _, artifact := range s.index {
		if s.matchesQuery(artifact, query) {
			results = append(results, artifact)
		}
		if query.Limit > 0 && len(results) >= query.Limit {
			break
		}
	}
	return results, nil
}

func (s *FileStore) Archive(ctx context.Context, artifactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	artifact, ok := s.index[artifactID]
	if !ok {
		return fmt.Errorf("artifact not found: %s", artifactID)
	}

	artifact.Status = StatusArchived
	artifact.UpdatedAt = time.Now()
	return s.saveIndex()
}

func (s *FileStore) matchesQuery(artifact *Artifact, query ArtifactQuery) bool {
	if query.SessionID != "" && artifact.SessionID != query.SessionID {
		return false
	}
	if query.Type != "" && artifact.Type != query.Type {
		return false
	}
	if query.Status != "" && artifact.Status != query.Status {
		return false
	}
	if query.CreatedBy != "" && artifact.CreatedBy != query.CreatedBy {
		return false
	}
	if len(query.Tags) > 0 {
		tagSet := make(map[string]bool)
		for _, t := range artifact.Tags {
			tagSet[t] = true
		}
		for _, t := range query.Tags {
			if !tagSet[t] {
				return false
			}
		}
	}
	return true
}

func (s *FileStore) loadIndex() error {
	indexPath := filepath.Join(s.basePath, "index.json")
	data, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read index: %w", err)
	}

	return json.Unmarshal(data, &s.index)
}

func (s *FileStore) saveIndex() error {
	indexPath := filepath.Join(s.basePath, "index.json")
	data, err := json.MarshalIndent(s.index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}
	return os.WriteFile(indexPath, data, 0644)
}
