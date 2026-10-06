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

func TestRangeGET(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	putB, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs", nil)
	resB, _ := http.DefaultClient.Do(putB)
	resB.Body.Close()

	payload := []byte("hello object store")
	put, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs/objects/a.txt", bytes.NewReader(payload))
	res, _ := http.DefaultClient.Do(put)
	res.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/buckets/logs/objects/a.txt", nil)
	req.Header.Set("Range", "bytes=0-4")
	got, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if got.StatusCode != http.StatusPartialContent {
		t.Fatalf("status %d", got.StatusCode)
	}
	if string(body) != "hello" {
		t.Fatalf("body %q", body)
	}
	if got.Header.Get("Content-Range") != "bytes 0-4/18" {
		t.Fatalf("content-range %s", got.Header.Get("Content-Range"))
	}

	bad, _ := http.NewRequest(http.MethodGet, srv.URL+"/buckets/logs/objects/a.txt", nil)
	bad.Header.Set("Range", "bytes=100-200")
	res416, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	res416.Body.Close()
	if res416.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 got %d", res416.StatusCode)
	}
}

func TestMultipartHTTP(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	putB, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs", nil)
	resB, _ := http.DefaultClient.Do(putB)
	resB.Body.Close()

	initRes, err := http.Post(srv.URL+"/buckets/logs/uploads?key=m.bin", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var up store.UploadInfo
	if err := json.NewDecoder(initRes.Body).Decode(&up); err != nil {
		t.Fatal(err)
	}
	initRes.Body.Close()
	if up.UploadID == "" {
		t.Fatal("no upload id")
	}

	p1, _ := http.NewRequest(http.MethodPut, srv.URL+"/uploads/"+up.UploadID+"/parts/1", bytes.NewReader([]byte("aa")))
	r1, _ := http.DefaultClient.Do(p1)
	r1.Body.Close()
	p2, _ := http.NewRequest(http.MethodPut, srv.URL+"/uploads/"+up.UploadID+"/parts/2", bytes.NewReader([]byte("bb")))
	r2, _ := http.DefaultClient.Do(p2)
	r2.Body.Close()

	done, err := http.Post(srv.URL+"/uploads/"+up.UploadID+"/complete", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	done.Body.Close()
	if done.StatusCode != http.StatusOK {
		t.Fatalf("complete %d", done.StatusCode)
	}
	get, _ := http.Get(srv.URL + "/buckets/logs/objects/m.bin")
	body, _ := io.ReadAll(get.Body)
	get.Body.Close()
	if string(body) != "aabb" {
		t.Fatalf("got %q", body)
	}

	init2, _ := http.Post(srv.URL+"/buckets/logs/uploads?key=z.bin", "application/json", nil)
	var up2 store.UploadInfo
	json.NewDecoder(init2.Body).Decode(&up2)
	init2.Body.Close()
	p, _ := http.NewRequest(http.MethodPut, srv.URL+"/uploads/"+up2.UploadID+"/parts/1", bytes.NewReader([]byte("x")))
	pr, _ := http.DefaultClient.Do(p)
	pr.Body.Close()
	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/uploads/"+up2.UploadID, nil)
	dr, _ := http.DefaultClient.Do(del)
	dr.Body.Close()
	if dr.StatusCode != http.StatusNoContent {
		t.Fatalf("abort %d", dr.StatusCode)
	}
}

func TestIfMatchIfNoneMatch(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	putB, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs", nil)
	resB, _ := http.DefaultClient.Do(putB)
	resB.Body.Close()

	put, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs/objects/k", bytes.NewReader([]byte("v1")))
	res, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	head, _ := http.Head(srv.URL + "/buckets/logs/objects/k")
	etag := head.Header.Get("ETag")
	head.Body.Close()

	none, _ := http.NewRequest(http.MethodGet, srv.URL+"/buckets/logs/objects/k", nil)
	none.Header.Set("If-None-Match", etag)
	nres, _ := http.DefaultClient.Do(none)
	nres.Body.Close()
	if nres.StatusCode != http.StatusNotModified {
		t.Fatalf("if-none-match get %d", nres.StatusCode)
	}

	badMatch, _ := http.NewRequest(http.MethodGet, srv.URL+"/buckets/logs/objects/k", nil)
	badMatch.Header.Set("If-Match", `"deadbeef"`)
	bres, _ := http.DefaultClient.Do(badMatch)
	bres.Body.Close()
	if bres.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("if-match get %d", bres.StatusCode)
	}

	star, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs/objects/k", bytes.NewReader([]byte("v2")))
	star.Header.Set("If-None-Match", "*")
	sres, _ := http.DefaultClient.Do(star)
	sres.Body.Close()
	if sres.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("if-none-match put %d", sres.StatusCode)
	}

	ok, _ := http.NewRequest(http.MethodPut, srv.URL+"/buckets/logs/objects/k", bytes.NewReader([]byte("v2")))
	ok.Header.Set("If-Match", etag)
	ores, _ := http.DefaultClient.Do(ok)
	ores.Body.Close()
	if ores.StatusCode != http.StatusOK {
		t.Fatalf("if-match put %d", ores.StatusCode)
	}
}
