package dictionary_test

import (
	"database/sql"
	"strings"
	"testing"

	"dict-manager/dictionary"

	_ "github.com/mattn/go-sqlite3"
)

func TestCatalogDictionaryTypes(t *testing.T) {
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

	tests := []struct {
		name       string
		dictionary dictionary.Name
		input      dictionary.EntryInput
		wantField  func(dictionary.Entry) string
		wantValue  string
		wantExport string
	}{
		{
			name:       "Chinese word",
			dictionary: dictionary.ChineseWords,
			input:      dictionary.EntryInput{Word: "中文", Reading: "zhong wen", Weight: 123, Category: 1},
			wantField:  func(entry dictionary.Entry) string { return entry.Reading },
			wantValue:  "zhong wen",
			wantExport: "中文\tzhong wen\t123\n",
		},
		{
			name:       "English word",
			dictionary: dictionary.EnglishWords,
			input:      dictionary.EntryInput{Word: "Spigot", Reading: "Spigot", Category: 8},
			wantField:  func(entry dictionary.Entry) string { return entry.Reading },
			wantValue:  "Spigot",
			wantExport: "Spigot\tSpigot\n",
		},
		{
			name:       "phrase",
			dictionary: dictionary.Phrases,
			input:      dictionary.EntryInput{Word: "快捷短语", Reading: "kj", Category: 4},
			wantField:  func(entry dictionary.Entry) string { return entry.Abbr },
			wantValue:  "kj",
			wantExport: "快捷短语\tkj\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid, err := catalog.Upsert(t.Context(), test.dictionary, []dictionary.EntryInput{test.input})
			if err != nil {
				t.Fatal(err)
			}
			if len(invalid) != 0 {
				t.Fatalf("valid input rejected: %q", invalid)
			}

			entries, err := catalog.List(t.Context(), test.dictionary, []int{test.input.Category})
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("entries = %d, want 1", len(entries))
			}
			if got := test.wantField(entries[0]); got != test.wantValue {
				t.Errorf("domain field = %q, want %q", got, test.wantValue)
			}

			export, err := catalog.Export(t.Context(), test.dictionary)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(export.Content, test.wantExport) {
				t.Errorf("export %q does not contain %q", export.Content, test.wantExport)
			}
		})
	}
}

func TestCategoriesReturnsCopy(t *testing.T) {
	first := dictionary.Categories()
	delete(first, 4)

	if got := dictionary.Categories()[4]; got != "Development" {
		t.Errorf("category 4 = %q, want Development", got)
	}
}
