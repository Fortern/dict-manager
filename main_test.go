package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"dict-manager/model"
	"dict-manager/store"
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
	if err := store.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	return (&apiServer{db: db}).routes()
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
	if addResponse.Header().Get("Content-Type") != "application/json" {
		t.Errorf("unexpected content type: %q", addResponse.Header().Get("Content-Type"))
	}

	listResponse := performRequest(handler, http.MethodGet, "/dicts/dict/en_words?category=8", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d; body = %s", listResponse.Code, listResponse.Body.String())
	}
	var words []model.EnWord
	if err := json.NewDecoder(listResponse.Body).Decode(&words); err != nil {
		t.Fatal(err)
	}
	if len(words) != 1 || words[0].Word != "Spigot" {
		t.Fatalf("unexpected words: %#v", words)
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

	form := url.Values{"id": {stringID(words[0].ID)}}.Encode()
	request := httptest.NewRequest(http.MethodDelete, "/dicts/dict/en_words", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("delete status = %d; body = %s", response.Code, body)
	}

	listResponse = performRequest(handler, http.MethodGet, "/dicts/dict/en_words?category=8", "")
	if listResponse.Code != http.StatusOK || strings.TrimSpace(listResponse.Body.String()) != "[]" {
		t.Errorf("expected empty list after delete; status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
}

func TestAddWordsRejectsInvalidBody(t *testing.T) {
	handler := newTestHandler(t)

	for _, body := range []string{"", `[{"word":"","reading":"x","category":1}]`} {
		response := performRequest(handler, http.MethodPost, "/dicts/dict/en_words", body)
		// TODO Incorrect testing. 返回的Code应为200，且json中应当包含不为空的error_words字段。
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
}

func stringID(id int) string {
	return strconv.Itoa(id)
}
