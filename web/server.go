// Package web exposes the dictionary domain over HTTP.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"dict-manager/dictionary"
)

// Catalog is the part of the dictionary domain consumed by the HTTP server.
type Catalog interface {
	List(context.Context, dictionary.Name, []int) ([]dictionary.Entry, error)
	Upsert(context.Context, dictionary.Name, []dictionary.EntryInput) ([]string, error)
	Delete(context.Context, dictionary.Name, int) error
	Export(context.Context, dictionary.Name) (dictionary.Export, error)
}

// Server serves the dictionary HTTP API.
type Server struct {
	catalog Catalog
	logger  *slog.Logger
}

// New creates an HTTP server for catalog.
func New(catalog Catalog, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{catalog: catalog, logger: logger}
}

// Handler returns the complete dictionary HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dicts/ping", s.ping)
	mux.HandleFunc("GET /dicts/dict/{dict_name}", s.listEntries)
	mux.HandleFunc("POST /dicts/dict/{dict_name}", s.upsertEntries)
	mux.HandleFunc("DELETE /dicts/dict/{dict_name}", s.deleteEntry)
	mux.HandleFunc("GET /dicts/export/{dict_name}", s.export)
	mux.HandleFunc("GET /dicts/category", s.listCategories)
	return s.requestLogger(s.recoverer(mux))
}

func (s *Server) ping(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "pong", "version": "1.1"})
}

func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	name, ok := s.dictionaryName(w, r)
	if !ok {
		return
	}
	category, err := strconv.Atoi(r.URL.Query().Get("category"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	entries, err := s.catalog.List(r.Context(), name, []int{category})
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, entries)
}

func (s *Server) upsertEntries(w http.ResponseWriter, r *http.Request) {
	name, ok := s.dictionaryName(w, r)
	if !ok {
		return
	}

	var inputs []dictionary.EntryInput
	if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	invalidWords, err := s.catalog.Upsert(r.Context(), name, inputs)
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string][]string{"error_words": invalidWords})
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request) {
	name, ok := s.dictionaryName(w, r)
	if !ok {
		return
	}

	reader := http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	id, err := strconv.Atoi(form.Get("id"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.catalog.Delete(r.Context(), name, id); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	name, ok := s.dictionaryName(w, r)
	if !ok {
		return
	}

	export, err := s.catalog.Export(r.Context(), name)
	if err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+export.Filename)
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, export.Content); err != nil {
		s.logger.Error("write export response", "error", err)
	}
}

func (s *Server) listCategories(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, dictionary.Categories())
}

func (s *Server) dictionaryName(w http.ResponseWriter, r *http.Request) (dictionary.Name, bool) {
	name, err := dictionary.ParseName(r.PathValue("dict_name"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return "", false
	}
	return name, true
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		s.logger.Error("write JSON response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.logger.Error("serve dictionary request", "error", err)
	s.writeError(w, http.StatusInternalServerError, err)
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info(
			"http request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				s.logger.Error(
					"panic serving HTTP request",
					"method", r.Method,
					"path", r.URL.Path,
					"error", value,
				)
				s.writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
