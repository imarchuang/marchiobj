package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
