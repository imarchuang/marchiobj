package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/marchi/marchiobj/store"
)

type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

func New(st *store.Store) *Server {
	s := &Server{store: st, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("PUT /buckets/{bucket}", s.handleCreateBucket)
	s.mux.HandleFunc("GET /buckets", s.handleListBuckets)
	s.mux.HandleFunc("PUT /buckets/{bucket}/objects/{key...}", s.handlePutObject)
	s.mux.HandleFunc("GET /buckets/{bucket}/objects/{key...}", s.handleGetObject)
	s.mux.HandleFunc("GET /buckets/{bucket}/objects", s.handleListObjects)
	s.mux.HandleFunc("HEAD /buckets/{bucket}/objects/{key...}", s.handleHeadObject)
	s.mux.HandleFunc("DELETE /buckets/{bucket}/objects/{key...}", s.handleDeleteObject)
	s.mux.HandleFunc("POST /buckets/{bucket}/uploads", s.handleInitMultipart)
	s.mux.HandleFunc("PUT /uploads/{id}/parts/{n}", s.handlePutPart)
	s.mux.HandleFunc("POST /uploads/{id}/complete", s.handleCompleteUpload)
	s.mux.HandleFunc("DELETE /uploads/{id}", s.handleAbortUpload)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateBucket(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("bucket")
	info, err := s.store.CreateBucket(name)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidBucket):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, store.ErrBucketExists):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (s *Server) handleListBuckets(w http.ResponseWriter, _ *http.Request) {
	list, err := s.store.ListBuckets()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []store.BucketInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": list})
}

func (s *Server) handleListObjects(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	q := r.URL.Query()
	res, err := s.store.ListObjects(bucket, q.Get("prefix"), q.Get("delimiter"), store.ParseMaxKeys(q.Get("max-keys")))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePutObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	meta, err := s.store.PutObjectCond(bucket, key, r.Body, store.CondFromHeader(r.Header))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleGetObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	meta, f, err := s.store.OpenObject(bucket, key)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	defer f.Close()
	if err := store.CheckReadCond(meta, store.CondFromHeader(r.Header)); err != nil {
		if errors.Is(err, store.ErrNotModified) {
			setObjectHeaders(w, meta)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		writeStoreError(w, err)
		return
	}

	start, end, hasRange, rangeErr := store.ParseByteRange(r.Header.Get("Range"), meta.Size)
	if hasRange && errors.Is(rangeErr, store.ErrRangeUnsatisfiable) {
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(meta.Size, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	setObjectHeaders(w, meta)
	w.Header().Set("Accept-Ranges", "bytes")
	if hasRange {
		n := end - start + 1
		w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(meta.Size, 10))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return
		}
		_, _ = io.CopyN(w, f, n)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func (s *Server) handleHeadObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	meta, err := s.store.HeadObject(bucket, key)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := store.CheckReadCond(meta, store.CondFromHeader(r.Header)); err != nil {
		if errors.Is(err, store.ErrNotModified) {
			setObjectHeaders(w, meta)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		writeStoreError(w, err)
		return
	}
	setObjectHeaders(w, meta)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeleteObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if err := s.store.DeleteObjectCond(bucket, key, store.CondFromHeader(r.Header)); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInitMultipart(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
	info, err := s.store.InitMultipart(bucket, key)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handlePutPart(w http.ResponseWriter, r *http.Request) {
	n, err := store.ParsePartNumber(r.PathValue("n"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	part, err := s.store.PutPart(r.PathValue("id"), n, r.Body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, part)
}

func (s *Server) handleCompleteUpload(w http.ResponseWriter, r *http.Request) {
	meta, err := s.store.CompleteUpload(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleAbortUpload(w http.ResponseWriter, r *http.Request) {
	if err := s.store.AbortUpload(r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func setObjectHeaders(w http.ResponseWriter, meta *store.ObjectMeta) {
	w.Header().Set("ETag", `"`+meta.ETag+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	w.Header().Set("Last-Modified", meta.MTime.UTC().Format(time.RFC1123))
	w.Header().Set("Content-Type", "application/octet-stream")
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalidBucket), errors.Is(err, store.ErrInvalidKey), errors.Is(err, store.ErrInvalidPart), errors.Is(err, store.ErrIncompleteMPU):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrBucketExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrBucketNotFound), errors.Is(err, store.ErrObjectNotFound), errors.Is(err, store.ErrUploadNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrPrecondition):
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrNotModified):
		w.WriteHeader(http.StatusNotModified)
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
