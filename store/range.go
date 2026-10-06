package store

import (
	"errors"
	"strconv"
	"strings"
)

var ErrRangeUnsatisfiable = errors.New("range not satisfiable")

// ParseByteRange parses a single HTTP Range value (bytes=start-end).
// A missing or malformed header returns ok=false so callers serve 200.
func ParseByteRange(header string, size int64) (start, end int64, ok bool, err error) {
	if header == "" {
		return 0, 0, false, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false, nil
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, 0, false, nil
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false, nil
	}
	left, right := spec[:dash], spec[dash+1:]
	if size == 0 {
		return 0, 0, true, ErrRangeUnsatisfiable
	}
	switch {
	case left == "" && right != "":
		suffix, err := strconv.ParseInt(right, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, nil
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true, nil
	case left != "" && right == "":
		start, err := strconv.ParseInt(left, 10, 64)
		if err != nil || start < 0 {
			return 0, 0, false, nil
		}
		if start >= size {
			return 0, 0, true, ErrRangeUnsatisfiable
		}
		return start, size - 1, true, nil
	case left != "" && right != "":
		start, err1 := strconv.ParseInt(left, 10, 64)
		end, err2 := strconv.ParseInt(right, 10, 64)
		if err1 != nil || err2 != nil || start < 0 || end < start {
			return 0, 0, false, nil
		}
		if start >= size {
			return 0, 0, true, ErrRangeUnsatisfiable
		}
		if end >= size {
			end = size - 1
		}
		return start, end, true, nil
	default:
		return 0, 0, false, nil
	}
}
