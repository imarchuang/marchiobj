package store

import "testing"

func TestParseByteRange(t *testing.T) {
	start, end, ok, err := ParseByteRange("bytes=0-4", 18)
	if !ok || err != nil || start != 0 || end != 4 {
		t.Fatalf("got %d-%d ok=%v err=%v", start, end, ok, err)
	}
	_, _, ok, err = ParseByteRange("bytes=100-200", 18)
	if !ok || err != ErrRangeUnsatisfiable {
		t.Fatalf("unsatisfiable: ok=%v err=%v", ok, err)
	}
	start, end, ok, err = ParseByteRange("bytes=10-", 18)
	if !ok || err != nil || start != 10 || end != 17 {
		t.Fatalf("open end %d-%d", start, end)
	}
}
