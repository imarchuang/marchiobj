package store

import "testing"

func TestCheckReadWriteCond(t *testing.T) {
	meta := &ObjectMeta{ETag: "abc"}
	if err := CheckReadCond(meta, Cond{IfNoneMatch: `"abc"`}); err != ErrNotModified {
		t.Fatalf("read if-none-match: %v", err)
	}
	if err := CheckReadCond(meta, Cond{IfMatch: `"zzz"`}); err != ErrPrecondition {
		t.Fatalf("read if-match: %v", err)
	}
	if err := CheckWriteCond(meta, Cond{IfNoneMatch: "*"}); err != ErrPrecondition {
		t.Fatalf("write if-none-match *: %v", err)
	}
	if err := CheckWriteCond(nil, Cond{IfMatch: `"abc"`}); err != ErrPrecondition {
		t.Fatalf("write if-match missing: %v", err)
	}
	if err := CheckWriteCond(meta, Cond{IfMatch: `"abc"`}); err != nil {
		t.Fatal(err)
	}
}
