package store

import (
	"net/http"
	"strings"
)

type Cond struct {
	IfMatch     string
	IfNoneMatch string
}

func CondFromHeader(h http.Header) Cond {
	return Cond{
		IfMatch:     h.Get("If-Match"),
		IfNoneMatch: h.Get("If-None-Match"),
	}
}

func normalizeETag(v string) string {
	v = strings.TrimSpace(v)
	return strings.Trim(v, `"`)
}

func etagStarOrEqual(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "*" {
		return true
	}
	return strings.EqualFold(normalizeETag(header), etag)
}

func CheckReadCond(meta *ObjectMeta, c Cond) error {
	if c.IfMatch != "" && !etagStarOrEqual(c.IfMatch, meta.ETag) {
		return ErrPrecondition
	}
	if c.IfNoneMatch != "" && etagStarOrEqual(c.IfNoneMatch, meta.ETag) {
		return ErrNotModified
	}
	return nil
}

func CheckWriteCond(meta *ObjectMeta, c Cond) error {
	exists := meta != nil
	if c.IfMatch != "" {
		if !exists || !etagStarOrEqual(c.IfMatch, meta.ETag) {
			return ErrPrecondition
		}
	}
	if c.IfNoneMatch != "" && exists && etagStarOrEqual(c.IfNoneMatch, meta.ETag) {
		return ErrPrecondition
	}
	return nil
}
