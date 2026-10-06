package store

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestPutGetDelete(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket("logs"); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello object")
	meta, err := st.PutObject("logs", "2026/a.txt", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Size != int64(len(payload)) || meta.ETag == "" {
		t.Fatalf("meta %+v", meta)
	}

	got, body, err := st.GetObject("logs", "2026/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if got.ETag != meta.ETag {
		t.Fatalf("etag %s vs %s", got.ETag, meta.ETag)
	}
	b, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, payload) {
		t.Fatalf("bytes %q", b)
	}

	head, err := st.HeadObject("logs", "2026/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if head.ETag != meta.ETag {
		t.Fatalf("head etag %s", head.ETag)
	}

	if err := st.DeleteObject("logs", "2026/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.GetObject("logs", "2026/a.txt"); err != ErrObjectNotFound {
		t.Fatalf("after delete: %v", err)
	}
}

func TestPutReplaceGCsOldBlob(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket("logs"); err != nil {
		t.Fatal(err)
	}
	m1, err := st.PutObject("logs", "k", bytes.NewReader([]byte("one")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject("logs", "k", bytes.NewReader([]byte("two!!"))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.blobPath("logs", m1.BlobID)); !os.IsNotExist(err) {
		t.Fatalf("old blob still present: %v", err)
	}
}
