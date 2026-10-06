package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var (
	ErrBucketExists   = errors.New("bucket already exists")
	ErrBucketNotFound = errors.New("bucket not found")
	ErrInvalidBucket  = errors.New("invalid bucket name")
	ErrInvalidKey     = errors.New("invalid object key")
	ErrObjectNotFound = errors.New("object not found")
	ErrUploadNotFound = errors.New("upload not found")
	ErrInvalidPart    = errors.New("invalid part number")
	ErrIncompleteMPU  = errors.New("incomplete multipart upload")
)

var bucketNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

type Store struct {
	dataDir string
	mu      sync.RWMutex
}

type BucketInfo struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "buckets"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "upload-index"), 0o755); err != nil {
		return nil, err
	}
	return &Store{dataDir: dataDir}, nil
}

func (s *Store) bucketDir(name string) string {
	return filepath.Join(s.dataDir, "buckets", name)
}

func ValidateBucket(name string) error {
	if !bucketNameRe.MatchString(name) {
		return ErrInvalidBucket
	}
	return nil
}

func (s *Store) CreateBucket(name string) (*BucketInfo, error) {
	if err := ValidateBucket(name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.bucketDir(name)
	metaPath := filepath.Join(dir, "bucket.json")
	if _, err := os.Stat(metaPath); err == nil {
		return nil, ErrBucketExists
	}

	for _, sub := range []string{"meta", "blobs", "multipart"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	info := &BucketInfo{Name: name, CreatedAt: time.Now().UTC()}
	if err := writeJSONAtomic(metaPath, info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *Store) ListBuckets() ([]BucketInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(filepath.Join(s.dataDir, "buckets"))
	if err != nil {
		return nil, err
	}
	out := make([]BucketInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.bucketDir(e.Name()), "bucket.json"))
		if err != nil {
			continue
		}
		var info BucketInfo
		if err := json.Unmarshal(b, &info); err != nil {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

func (s *Store) BucketExists(name string) bool {
	_, err := os.Stat(filepath.Join(s.bucketDir(name), "bucket.json"))
	return err == nil
}

func writeJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
