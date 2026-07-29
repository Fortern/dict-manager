package web_test

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"dict-manager/dictionary"
	"dict-manager/web"

	_ "github.com/mattn/go-sqlite3"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	catalog := dictionary.NewCatalog(db)
	if err := catalog.InitSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return web.New(catalog, logger).Handler()
}

func performRequest(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestRoutes(t *testing.T) {
	handler := newTestHandler(t)

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantBody   string
	}{
		{name: "api ping", method: http.MethodGet, target: "/dicts/ping", wantStatus: http.StatusOK, wantBody: `"version":"1.1"`},
		{name: "categories", method: http.MethodGet, target: "/dicts/category", wantStatus: http.StatusOK, wantBody: `"Development"`},
		{name: "invalid dictionary", method: http.MethodGet, target: "/dicts/dict/unknown?category=1", wantStatus: http.StatusBadRequest, wantBody: `"error"`},
		{name: "invalid category", method: http.MethodGet, target: "/dicts/dict/en_words?category=abc", wantStatus: http.StatusBadRequest, wantBody: `"error"`},
		{name: "method not allowed", method: http.MethodPut, target: "/dicts/category", wantStatus: http.StatusMethodNotAllowed, wantBody: "Method Not Allowed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(handler, test.method, test.target, "")
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Errorf("body %q does not contain %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestEnglishWordLifecycle(t *testing.T) {
	handler := newTestHandler(t)

	addResponse := performRequest(
		handler,
		http.MethodPost,
		"/dicts/dict/en_words",
		`[{"word":"Spigot","reading":"Spigot","category":8}]`,
	)
	if addResponse.Code != http.StatusOK {
		t.Fatalf("add status = %d; body = %s", addResponse.Code, addResponse.Body.String())
	}
	mediaType, _, err := mime.ParseMediaType(addResponse.Header().Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "application/json" {
		t.Errorf("unexpected content type: %q", mediaType)
	}

	listResponse := performRequest(handler, http.MethodGet, "/dicts/dict/en_words?category=8", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d; body = %s", listResponse.Code, listResponse.Body.String())
	}
	var entries []dictionary.Entry
	if err := json.NewDecoder(listResponse.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Word != "Spigot" {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	exportResponse := performRequest(handler, http.MethodGet, "/dicts/export/en_words", "")
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("export status = %d; body = %s", exportResponse.Code, exportResponse.Body.String())
	}
	if got := exportResponse.Header().Get("Content-Disposition"); got != "attachment; filename=common_en.dict.yaml" {
		t.Errorf("unexpected content disposition: %q", got)
	}
	if !strings.Contains(exportResponse.Body.String(), "Spigot\tSpigot\n") {
		t.Errorf("unexpected export body: %q", exportResponse.Body.String())
	}

	form := url.Values{"id": {strconv.Itoa(entries[0].ID)}}.Encode()
	request := httptest.NewRequest(http.MethodDelete, "/dicts/dict/en_words", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status = %d; body = %s", response.Code, response.Body.String())
	}

	listResponse = performRequest(handler, http.MethodGet, "/dicts/dict/en_words?category=8", "")
	if listResponse.Code != http.StatusOK || strings.TrimSpace(listResponse.Body.String()) != "[]" {
		t.Errorf("expected empty list after delete; status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
}

func TestUpsertValidation(t *testing.T) {
	handler := newTestHandler(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "malformed JSON", body: "", wantStatus: http.StatusBadRequest, wantBody: `"error"`},
		{
			name:       "invalid word",
			body:       `[{"word":"","reading":"x","category":1}]`,
			wantStatus: http.StatusOK,
			wantBody:   `"error_words":[""]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(handler, http.MethodPost, "/dicts/dict/en_words", test.body)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Errorf("body %q does not contain %q", response.Body.String(), test.wantBody)
			}
		})
	}
}
