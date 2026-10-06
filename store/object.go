package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type ObjectMeta struct {
	Key    string    `json:"key"`
	BlobID string    `json:"blobId"`
	Size   int64     `json:"size"`
	ETag   string    `json:"etag"`
	MTime  time.Time `json:"mtime"`
}

func ValidateKey(key string) error {
	if key == "" || !utf8.ValidString(key) || len(key) > 1024 {
		return ErrInvalidKey
	}
	if strings.ContainsRune(key, 0) {
		return ErrInvalidKey
	}
	return nil
}

func (s *Store) metaPath(bucket, key string) string {
	return filepath.Join(s.bucketDir(bucket), "meta", url.PathEscape(key)+".json")
}

func (s *Store) blobPath(bucket, blobID string) string {
	return filepath.Join(s.bucketDir(bucket), "blobs", blobID)
}

func (s *Store) PutObject(bucket, key string, r io.Reader) (*ObjectMeta, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.bucketExistsLocked(bucket) {
		return nil, ErrBucketNotFound
	}

	blobDir := filepath.Join(s.bucketDir(bucket), "blobs")
	tmp, err := os.CreateTemp(blobDir, ".put-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return nil, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	dest := s.blobPath(bucket, sum)
	if _, err := os.Stat(dest); err == nil {
		os.Remove(tmpName)
	} else if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return nil, err
	}

	old, _ := s.readMetaLocked(bucket, key)
	meta := &ObjectMeta{
		Key:    key,
		BlobID: sum,
		Size:   n,
		ETag:   sum,
		MTime:  time.Now().UTC(),
	}
	if err := writeJSONAtomic(s.metaPath(bucket, key), meta); err != nil {
		return nil, err
	}
	if old != nil && old.BlobID != sum {
		s.gcBlobLocked(bucket, old.BlobID)
	}
	return meta, nil
}

func (s *Store) HeadObject(bucket, key string) (*ObjectMeta, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.bucketExistsLocked(bucket) {
		return nil, ErrBucketNotFound
	}
	meta, err := s.readMetaLocked(bucket, key)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *Store) GetObject(bucket, key string) (*ObjectMeta, io.ReadCloser, error) {
	meta, f, err := s.OpenObject(bucket, key)
	if err != nil {
		return nil, nil, err
	}
	return meta, f, nil
}

func (s *Store) OpenObject(bucket, key string) (*ObjectMeta, *os.File, error) {
	if err := ValidateKey(key); err != nil {
		return nil, nil, err
	}
	s.mu.RLock()
	if !s.bucketExistsLocked(bucket) {
		s.mu.RUnlock()
		return nil, nil, ErrBucketNotFound
	}
	meta, err := s.readMetaLocked(bucket, key)
	if err != nil {
		s.mu.RUnlock()
		return nil, nil, err
	}
	f, err := os.Open(s.blobPath(bucket, meta.BlobID))
	s.mu.RUnlock()
	if err != nil {
		return nil, nil, err
	}
	return meta, f, nil
}

func (s *Store) DeleteObject(bucket, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.bucketExistsLocked(bucket) {
		return ErrBucketNotFound
	}
	meta, err := s.readMetaLocked(bucket, key)
	if err != nil {
		return err
	}
	if err := os.Remove(s.metaPath(bucket, key)); err != nil {
		return err
	}
	s.gcBlobLocked(bucket, meta.BlobID)
	return nil
}

func (s *Store) bucketExistsLocked(name string) bool {
	_, err := os.Stat(filepath.Join(s.bucketDir(name), "bucket.json"))
	return err == nil
}

func (s *Store) readMetaLocked(bucket, key string) (*ObjectMeta, error) {
	b, err := os.ReadFile(s.metaPath(bucket, key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	var meta ObjectMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *Store) gcBlobLocked(bucket, blobID string) {
	entries, err := os.ReadDir(filepath.Join(s.bucketDir(bucket), "meta"))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.bucketDir(bucket), "meta", e.Name()))
		if err != nil {
			continue
		}
		var meta ObjectMeta
		if json.Unmarshal(b, &meta) == nil && meta.BlobID == blobID {
			return
		}
	}
	_ = os.Remove(s.blobPath(bucket, blobID))
}
