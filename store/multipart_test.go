package store

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMultipartCompleteAndAbort(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket("logs"); err != nil {
		t.Fatal(err)
	}

	up, err := st.InitMultipart("logs", "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutPart(up.UploadID, 1, bytes.NewReader([]byte("aaa"))); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutPart(up.UploadID, 2, bytes.NewReader([]byte("bbb"))); err != nil {
		t.Fatal(err)
	}
	meta, err := st.CompleteUpload(up.UploadID)
	if err != nil {
		t.Fatal(err)
	}
	body, rc, err := st.GetObject("logs", "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if body.Size != 6 || string(got) != "aaabbb" {
		t.Fatalf("got %q meta %+v", got, meta)
	}
	listed, err := st.ListObjects("logs", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Objects) != 1 {
		t.Fatalf("list %+v", listed.Objects)
	}

	up2, err := st.InitMultipart("logs", "other.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutPart(up2.UploadID, 1, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	if err := st.AbortUpload(up2.UploadID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.uploadDir("logs", up2.UploadID)); !os.IsNotExist(err) {
		t.Fatalf("abort left parts: %v", err)
	}
	if _, err := st.HeadObject("logs", "other.bin"); err != ErrObjectNotFound {
		t.Fatalf("aborted object visible: %v", err)
	}
}

func TestCompleteAtomicNoHalfObject(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket("logs"); err != nil {
		t.Fatal(err)
	}
	up, err := st.InitMultipart("logs", "half.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutPart(up.UploadID, 1, bytes.NewReader([]byte("one"))); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutPart(up.UploadID, 2, bytes.NewReader([]byte("two"))); err != nil {
		t.Fatal(err)
	}
	if _, err := st.completeUpload(up.UploadID, completeOpts{failDuringConcat: true}); err == nil {
		t.Fatal("expected simulated crash")
	}
	listed, err := st.ListObjects("logs", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Objects) != 0 {
		t.Fatalf("half object listed: %+v", listed.Objects)
	}

	up3, err := st.InitMultipart("logs", "half2.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutPart(up3.UploadID, 1, bytes.NewReader([]byte("aa"))); err != nil {
		t.Fatal(err)
	}
	if _, err := st.completeUpload(up3.UploadID, completeOpts{skipPublish: true}); err != nil {
		t.Fatal(err)
	}
	listed2, err := st.ListObjects("logs", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed2.Objects) != 0 {
		t.Fatalf("unpublished concat listed: %+v", listed2.Objects)
	}
	entries, _ := os.ReadDir(st.bucketDir("logs") + "/blobs")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".complete-") {
			t.Fatalf("tmp concat leaked as listed name: %s", e.Name())
		}
	}
}
