package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ListResult struct {
	Prefix         string       `json:"prefix"`
	Delimiter      string       `json:"delimiter,omitempty"`
	MaxKeys        int          `json:"maxKeys"`
	Objects        []ObjectMeta `json:"objects"`
	CommonPrefixes []string     `json:"commonPrefixes"`
}

func (s *Store) ListObjects(bucket, prefix, delimiter string, maxKeys int) (*ListResult, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.bucketExistsLocked(bucket) {
		return nil, ErrBucketNotFound
	}

	entries, err := os.ReadDir(filepath.Join(s.bucketDir(bucket), "meta"))
	if err != nil {
		return nil, err
	}
	keys := make([]ObjectMeta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.bucketDir(bucket), "meta", e.Name()))
		if err != nil {
			continue
		}
		var meta ObjectMeta
		if json.Unmarshal(b, &meta) != nil {
			continue
		}
		if !strings.HasPrefix(meta.Key, prefix) {
			continue
		}
		keys = append(keys, meta)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })

	out := &ListResult{
		Prefix:         prefix,
		Delimiter:      delimiter,
		MaxKeys:        maxKeys,
		Objects:        []ObjectMeta{},
		CommonPrefixes: []string{},
	}
	seenPrefix := map[string]bool{}
	for _, meta := range keys {
		if len(out.Objects)+len(out.CommonPrefixes) >= maxKeys {
			break
		}
		rest := meta.Key[len(prefix):]
		if delimiter != "" {
			if i := strings.Index(rest, delimiter); i >= 0 {
				cp := prefix + rest[:i+len(delimiter)]
				if !seenPrefix[cp] {
					seenPrefix[cp] = true
					out.CommonPrefixes = append(out.CommonPrefixes, cp)
				}
				continue
			}
		}
		out.Objects = append(out.Objects, meta)
	}
	return out, nil
}

func ParseMaxKeys(s string) int {
	if s == "" {
		return 1000
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 1000
	}
	return n
}
