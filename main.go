package main

import (
	"database/sql"
	"dict-manager/model"
	"dict-manager/store"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type apiServer struct {
	db *sql.DB
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write json response", "msg", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func dictNameFromRequest(r *http.Request) string {
	return model.GetDictName(r.PathValue("dict_name"))
}

func (s *apiServer) listWordsHandler(w http.ResponseWriter, r *http.Request) {
	dictName := dictNameFromRequest(r)
	if dictName == "" {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	category, err := strconv.Atoi(r.URL.Query().Get("category"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var errorWords any
	if dictName == "cn_words" {
		errorWords, err = store.GetCnWords(s.db, []int{category})
	} else if dictName == "en_words" {
		errorWords, err = store.GetEnWords(s.db, []int{category})
	} else if dictName == "phrases" {
		errorWords, err = store.GetPhrases(s.db, []int{category})
	} else {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	if err != nil {
		slog.Error(err.Error())
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, errorWords)
}

func (s *apiServer) addWordsHandler(w http.ResponseWriter, r *http.Request) {
	dictName := dictNameFromRequest(r)
	if dictName == "" {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	var request []model.WordItem
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var errorWords []string
	var err error
	if dictName == "cn_words" {
		errorWords, err = store.UpsertCnWords(s.db, request)
	} else if dictName == "en_words" {
		errorWords, err = store.UpsertEnWords(s.db, request)
	} else if dictName == "phrases" {
		errorWords, err = store.UpsertPhrases(s.db, request)
	} else {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	if err != nil {
		slog.Error(err.Error())
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"error_words": errorWords})
}

func (s *apiServer) deleteWordHandler(w http.ResponseWriter, r *http.Request) {
	dictName := dictNameFromRequest(r)
	if dictName == "" {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	reader := http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id, err := strconv.Atoi(form.Get("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if dictName == "cn_words" {
		err = store.DeleteFromCnWordsById(s.db, id)
	} else if dictName == "en_words" {
		err = store.DeleteFromEnWordsById(s.db, id)
	} else if dictName == "phrases" {
		err = store.DeleteFromPhrasesById(s.db, id)
	} else {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	if err != nil {
		slog.Error(err.Error())
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *apiServer) exportHandler(w http.ResponseWriter, r *http.Request) {
	dictName := dictNameFromRequest(r)
	if dictName == "" {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	var lines string
	var err error
	var fileName string
	if dictName == "cn_words" {
		lines, err = exportCnWords(s.db)
		fileName = "common.dict.yaml"
	} else if dictName == "en_words" {
		lines, err = exportEnWords(s.db)
		fileName = "common_en.dict.yaml"
	} else if dictName == "phrases" {
		lines, err = exportPhrases(s.db)
		fileName = "custom_phrase.txt"
	} else {
		writeError(w, http.StatusBadRequest, errors.New("path_param 'dict_name' is invalid"))
		return
	}
	if err != nil {
		slog.Error(err.Error())
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, lines); err != nil {
		slog.Error("write export response", "msg", err)
	}
}

func exportCnWords(db *sql.DB) (string, error) {
	words, err := store.GetCnWords(db, nil)
	if err != nil {
		return "", err
	}
	lines := strings.Builder{}
	lines.WriteString("# Rime dictionary\n# encoding: utf-8\n---\nname: common\nversion: \"")
	lines.WriteString(strconv.FormatInt(time.Now().Unix(), 10))
	lines.WriteString("\"\nsort: by_weight\n...\n")
	for _, word := range words {
		lines.WriteString(word.Word)
		lines.WriteString("\t")
		lines.WriteString(word.Reading)
		lines.WriteString("\t")
		lines.WriteString(strconv.Itoa(word.Weight))
		lines.WriteString("\n")
	}
	return lines.String(), nil
}

func exportEnWords(db *sql.DB) (string, error) {
	words, err := store.GetEnWords(db, nil)
	if err != nil {
		return "", err
	}
	lines := strings.Builder{}
	lines.WriteString("# Rime dictionary\n# encoding: utf-8\n---\nname: common_en\nversion: \"")
	lines.WriteString(strconv.FormatInt(time.Now().Unix(), 10))
	lines.WriteString("\"\nsort: by_weight\n...\n")
	for _, word := range words {
		lines.WriteString(word.Word)
		lines.WriteString("\t")
		lines.WriteString(word.Reading)
		lines.WriteString("\n")
	}
	return lines.String(), nil
}

func exportPhrases(db *sql.DB) (string, error) {
	words, err := store.GetPhrases(db, nil)
	if err != nil {
		return "", err
	}
	lines := strings.Builder{}
	for _, word := range words {
		lines.WriteString(word.Word)
		lines.WriteString("\t")
		lines.WriteString(word.Abbr)
		lines.WriteString("\n")
	}
	return lines.String(), nil
}

func getCategoriesHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, model.GetCategories())
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dicts/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "pong", "version": "1.1"})
	})
	mux.HandleFunc("GET /dicts/dict/{dict_name}", s.listWordsHandler)
	mux.HandleFunc("POST /dicts/dict/{dict_name}", s.addWordsHandler)
	mux.HandleFunc("DELETE /dicts/dict/{dict_name}", s.deleteWordHandler)
	mux.HandleFunc("GET /dicts/export/{dict_name}", s.exportHandler)
	mux.HandleFunc("GET /dicts/category", getCategoriesHandler)
	return requestLogger(recoverer(mux))
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				slog.Error("panic serving http request", "method", r.Method, "path", r.URL.Path, "error", value)
				writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func main() {
	db, err := sql.Open("sqlite3", "dict.db")
	if err != nil {
		slog.Error("open sqlite store error", "msg", err)
		return
	}
	defer func(db *sql.DB) {
		e := db.Close()
		if e != nil {
			slog.Error("Error closing sqlite store.", "msg", err)
		}
	}(db)

	if err = store.InitSchema(db); err != nil {
		slog.Error("init store error.", "msg", err)
		return
	}

	server := &http.Server{
		Addr:              ":8080",
		Handler:           (&apiServer{db: db}).routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("starting server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("start server error", "msg", err)
	}
}
