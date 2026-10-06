package httpapi

import (
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
