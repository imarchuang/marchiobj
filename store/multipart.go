package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type UploadInfo struct {
	UploadID string    `json:"uploadId"`
	Bucket   string    `json:"bucket"`
	Key      string    `json:"key"`
	Created  time.Time `json:"createdAt"`
}

type PartInfo struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
}

func (s *Store) uploadDir(bucket, id string) string {
	return filepath.Join(s.bucketDir(bucket), "multipart", id)
}

func (s *Store) uploadIndexPath(id string) string {
	return filepath.Join(s.dataDir, "upload-index", id+".json")
}

func (s *Store) InitMultipart(bucket, key string) (*UploadInfo, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.bucketExistsLocked(bucket) {
		return nil, ErrBucketNotFound
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(raw[:])
	info := &UploadInfo{UploadID: id, Bucket: bucket, Key: key, Created: time.Now().UTC()}
	if err := os.MkdirAll(s.uploadDir(bucket, id), 0o755); err != nil {
		return nil, err
	}
	if err := writeJSONAtomic(filepath.Join(s.uploadDir(bucket, id), "meta.json"), info); err != nil {
		return nil, err
	}
	if err := writeJSONAtomic(s.uploadIndexPath(id), info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *Store) PutPart(uploadID string, n int, r io.Reader) (*PartInfo, error) {
	if n < 1 {
		return nil, ErrInvalidPart
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.loadUploadLocked(uploadID)
	if err != nil {
		return nil, err
	}
	partPath := filepath.Join(s.uploadDir(info.Bucket, uploadID), fmt.Sprintf("part-%05d", n))
	tmp, err := os.CreateTemp(s.uploadDir(info.Bucket, uploadID), ".part-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), r)
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
	if err := os.Rename(tmpName, partPath); err != nil {
		os.Remove(tmpName)
		return nil, err
	}
	return &PartInfo{PartNumber: n, ETag: hex.EncodeToString(h.Sum(nil)), Size: size}, nil
}

func (s *Store) CompleteUpload(uploadID string) (*ObjectMeta, error) {
	return s.completeUpload(uploadID, completeOpts{})
}

type completeOpts struct {
	failDuringConcat bool
	skipPublish      bool
}

func (s *Store) completeUpload(uploadID string, opts completeOpts) (*ObjectMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.loadUploadLocked(uploadID)
	if err != nil {
		return nil, err
	}
	parts, err := s.listPartsLocked(info.Bucket, uploadID)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, ErrIncompleteMPU
	}
	for i, p := range parts {
		if p != i+1 {
			return nil, ErrIncompleteMPU
		}
	}

	blobDir := filepath.Join(s.bucketDir(info.Bucket), "blobs")
	tmp, err := os.CreateTemp(blobDir, ".complete-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	h := sha256.New()
	var total int64
	for i, pn := range parts {
		if opts.failDuringConcat && i == 1 {
			tmp.Close()
			os.Remove(tmpName)
			return nil, fmt.Errorf("simulated crash during concat")
		}
		pf, err := os.Open(filepath.Join(s.uploadDir(info.Bucket, uploadID), fmt.Sprintf("part-%05d", pn)))
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return nil, err
		}
		n, err := io.Copy(io.MultiWriter(tmp, h), pf)
		pf.Close()
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return nil, err
		}
		total += n
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
	dest := s.blobPath(info.Bucket, sum)
	if _, err := os.Stat(dest); err == nil {
		os.Remove(tmpName)
	} else if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return nil, err
	}

	if opts.skipPublish {
		return nil, nil
	}

	old, _ := s.readMetaLocked(info.Bucket, info.Key)
	meta := &ObjectMeta{
		Key:    info.Key,
		BlobID: sum,
		Size:   total,
		ETag:   sum,
		MTime:  time.Now().UTC(),
	}
	if err := writeJSONAtomic(s.metaPath(info.Bucket, info.Key), meta); err != nil {
		return nil, err
	}
	if old != nil && old.BlobID != sum {
		s.gcBlobLocked(info.Bucket, old.BlobID)
	}
	s.removeUploadLocked(info)
	return meta, nil
}

func (s *Store) AbortUpload(uploadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.loadUploadLocked(uploadID)
	if err != nil {
		return err
	}
	s.removeUploadLocked(info)
	return nil
}

func (s *Store) loadUploadLocked(id string) (*UploadInfo, error) {
	b, err := os.ReadFile(s.uploadIndexPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrUploadNotFound
		}
		return nil, err
	}
	var info UploadInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *Store) listPartsLocked(bucket, id string) ([]int, error) {
	entries, err := os.ReadDir(s.uploadDir(bucket, id))
	if err != nil {
		return nil, err
	}
	var nums []int
	for _, e := range entries {
		var n int
		if _, err := fmt.Sscanf(e.Name(), "part-%05d", &n); err == nil {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	return nums, nil
}

func (s *Store) removeUploadLocked(info *UploadInfo) {
	_ = os.RemoveAll(s.uploadDir(info.Bucket, info.UploadID))
	_ = os.Remove(s.uploadIndexPath(info.UploadID))
}

func ParsePartNumber(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, ErrInvalidPart
	}
	return n, nil
}
