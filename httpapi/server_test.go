package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/marchi/marchiobj/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(st)
}

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["status"] != "ok" {
		t.Fatalf("got %q", m["status"])
	}
}

func TestCreateBucket(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", res.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs", nil)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status %d", res2.StatusCode)
	}

	if _, err := os.Stat(filepath.Join(dir, "buckets", "logs", "bucket.json")); err != nil {
		t.Fatalf("bucket.json missing: %v", err)
	}

	listRes, err := http.Get(srv.URL + "/buckets")
	if err != nil {
		t.Fatal(err)
	}
	defer listRes.Body.Close()
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("list status %d", listRes.StatusCode)
	}
	var payload struct {
		Buckets []store.BucketInfo `json:"buckets"`
	}
	if err := json.NewDecoder(listRes.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Buckets) != 1 || payload.Buckets[0].Name != "logs" {
		t.Fatalf("list: %+v", payload.Buckets)
	}
}

func TestPutGetHeadDeleteObject(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	putB, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/photos", nil)
	resB, err := http.DefaultClient.Do(putB)
	if err != nil {
		t.Fatal(err)
	}
	resB.Body.Close()
	if resB.StatusCode != http.StatusCreated {
		t.Fatalf("bucket %d", resB.StatusCode)
	}

	payload := []byte("hello object store")
	put, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/photos/objects/2026/a.txt", bytes.NewReader(payload))
	res, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("put %d %s", res.StatusCode, body)
	}

	get, err := http.Get(srv.URL + "/buckets/photos/objects/2026/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(get.Body)
	get.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if get.StatusCode != http.StatusOK {
		t.Fatalf("get %d", get.StatusCode)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("bytes %q", got)
	}
	etag := get.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing etag")
	}

	head, err := http.Head(srv.URL + "/buckets/photos/objects/2026/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	head.Body.Close()
	if head.StatusCode != http.StatusOK {
		t.Fatalf("head %d", head.StatusCode)
	}
	if head.Header.Get("ETag") != etag {
		t.Fatalf("head etag %s vs %s", head.Header.Get("ETag"), etag)
	}
	if head.ContentLength != int64(len(payload)) {
		t.Fatalf("head length %d", head.ContentLength)
	}

	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/buckets/photos/objects/2026/a.txt", nil)
	delRes, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	delRes.Body.Close()
	if delRes.StatusCode != http.StatusNoContent {
		t.Fatalf("delete %d", delRes.StatusCode)
	}

	missing, err := http.Get(srv.URL + "/buckets/photos/objects/2026/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete %d", missing.StatusCode)
	}
}

func TestCreateBucketInvalid(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/Bad_Name", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestListObjectsHTTP(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	putB, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/pics", nil)
	resB, _ := http.DefaultClient.Do(putB)
	resB.Body.Close()

	for _, k := range []string{"photos/a.jpg", "photos/b.jpg", "docs/x"} {
		put, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/pics/objects/"+k, bytes.NewReader([]byte(k)))
		res, err := http.DefaultClient.Do(put)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("put %s %d", k, res.StatusCode)
		}
	}

	res, err := http.Get(srv.URL + "/buckets/pics/objects?prefix=photos/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var listed store.ListResult
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Objects) != 2 {
		t.Fatalf("prefix list %+v", listed)
	}

	res2, err := http.Get(srv.URL + "/buckets/pics/objects?delimiter=/")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	var delim store.ListResult
	if err := json.NewDecoder(res2.Body).Decode(&delim); err != nil {
		t.Fatal(err)
	}
	if len(delim.CommonPrefixes) != 2 {
		t.Fatalf("delimiter %+v", delim)
	}
}
