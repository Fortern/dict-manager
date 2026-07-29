package dictionary

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const currentSchemaVersion = 100

const createSchemaMetaTable = `
	CREATE TABLE IF NOT EXISTS schema_meta (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		value INTEGER NOT NULL,
		applied_at INTEGER DEFAULT (unixepoch())
	);
`

const createDictionaryTables = `
	CREATE TABLE IF NOT EXISTS cn_words (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		word TEXT NOT NULL UNIQUE,
		reading TEXT NOT NULL,
		weight INTEGER NOT NULL DEFAULT 10,
		category INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS cn_words_category_idx ON cn_words (category);
	CREATE TABLE IF NOT EXISTS en_words (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		word TEXT NOT NULL UNIQUE,
		reading TEXT NOT NULL,
		category INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS en_words_category_idx ON en_words (category);
	CREATE TABLE IF NOT EXISTS phrases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		word TEXT NOT NULL UNIQUE,
		reading TEXT NOT NULL,
		category INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS phrases_category_idx ON phrases (category);
`

const (
	upsertChineseWord = `
		INSERT INTO cn_words(word, reading, weight, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(word) DO UPDATE SET
			reading = excluded.reading,
			weight = excluded.weight,
			category = excluded.category,
			updated_at = excluded.updated_at;
	`
	upsertEnglishWord = `
		INSERT INTO en_words(word, reading, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(word) DO UPDATE SET
			reading = excluded.reading,
			category = excluded.category,
			updated_at = excluded.updated_at;
	`
	upsertPhrase = `
		INSERT INTO phrases(word, reading, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(word) DO UPDATE SET
			reading = excluded.reading,
			category = excluded.category,
			updated_at = excluded.updated_at;
	`
)

// Catalog is the application's dictionary collection backed by SQLite.
type Catalog struct {
	db *sql.DB
}

// NewCatalog creates a dictionary catalog using db.
func NewCatalog(db *sql.DB) *Catalog {
	return &Catalog{db: db}
}

// InitSchema prepares the dictionary tables.
func (c *Catalog) InitSchema(ctx context.Context) error {
	return c.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, createSchemaMetaTable); err != nil {
			return fmt.Errorf("create schema metadata: %w", err)
		}

		var version int
		err := tx.QueryRowContext(
			ctx,
			"SELECT value FROM schema_meta WHERE name = 'schema_ver'",
		).Scan(&version)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(
				ctx,
				"INSERT INTO schema_meta (name, value) VALUES (?, ?)",
				"schema_ver",
				currentSchemaVersion,
			); err != nil {
				return fmt.Errorf("record schema version: %w", err)
			}
			if _, err := tx.ExecContext(ctx, createDictionaryTables); err != nil {
				return fmt.Errorf("create dictionary tables: %w", err)
			}
		case err != nil:
			return fmt.Errorf("read schema version: %w", err)
		default:
			if _, err := tx.ExecContext(
				ctx,
				"UPDATE schema_meta SET value = ? WHERE name = ?",
				currentSchemaVersion,
				"schema_ver",
			); err != nil {
				return fmt.Errorf("update schema version: %w", err)
			}
		}
		return nil
	})
}

// List returns entries from name, optionally filtered by category.
func (c *Catalog) List(ctx context.Context, name Name, categories []int) ([]Entry, error) {
	query, err := selectQuery(name)
	if err != nil {
		return nil, err
	}

	args := make([]any, len(categories))
	if len(categories) > 0 {
		placeholders := make([]string, len(categories))
		for i, category := range categories {
			placeholders[i] = "?"
			args[i] = category
		}
		query += " WHERE category IN (" + strings.Join(placeholders, ",") + ")"
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", name, err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	entries := make([]Entry, 0)
	for rows.Next() {
		var entry Entry
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&entry.ID,
			&entry.Word,
			&entry.Reading,
			&entry.Abbr,
			&entry.Weight,
			&entry.Category,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan %s entry: %w", name, err)
		}
		entry.CreatedAt = time.Unix(createdAt, 0)
		entry.UpdatedAt = time.Unix(updatedAt, 0)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s entries: %w", name, err)
	}
	return entries, nil
}

// Upsert adds or updates entries. Invalid words are returned separately and do
// not prevent valid words in the same request from being stored.
func (c *Catalog) Upsert(ctx context.Context, name Name, inputs []EntryInput) ([]string, error) {
	query, err := upsertQuery(name)
	if err != nil {
		return nil, err
	}

	invalidWords := make([]string, 0)
	err = c.withTx(ctx, func(tx *sql.Tx) error {
		statement, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("prepare %s upsert: %w", name, err)
		}
		defer func(statement *sql.Stmt) {
			_ = statement.Close()
		}(statement)

		now := time.Now().Unix()
		for _, input := range inputs {
			input.Word = strings.Trim(input.Word, " ")
			input.Reading = strings.Trim(input.Reading, " ")
			if !validInput(name, input) {
				invalidWords = append(invalidWords, input.Word)
				continue
			}

			var args []any
			if name == ChineseWords {
				if input.Weight <= 0 {
					input.Weight = 999
				}
				args = []any{input.Word, input.Reading, input.Weight, input.Category, now, now}
			} else {
				args = []any{input.Word, input.Reading, input.Category, now, now}
			}
			if _, err := statement.ExecContext(ctx, args...); err != nil {
				return fmt.Errorf("upsert %s word %q: %w", name, input.Word, err)
			}
		}
		return nil
	})
	return invalidWords, err
}

// Delete removes an entry by ID.
func (c *Catalog) Delete(ctx context.Context, name Name, id int) error {
	table, err := tableName(name)
	if err != nil {
		return err
	}
	if _, err := c.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE id = ?", id); err != nil {
		return fmt.Errorf("cannot delete from %s: %w", name, err)
	}
	return nil
}

func (c *Catalog) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func(tx *sql.Tx) {
		_ = tx.Rollback()
	}(tx)

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func selectQuery(name Name) (string, error) {
	switch name {
	case ChineseWords:
		return `SELECT id, word, reading, '', weight, category, created_at, updated_at FROM cn_words`, nil
	case EnglishWords:
		return `SELECT id, word, reading, '', 0, category, created_at, updated_at FROM en_words`, nil
	case Phrases:
		return `SELECT id, word, '', reading, 0, category, created_at, updated_at FROM phrases`, nil
	default:
		return "", fmt.Errorf("list %q: invalid dictionary", name)
	}
}

func upsertQuery(name Name) (string, error) {
	switch name {
	case ChineseWords:
		return upsertChineseWord, nil
	case EnglishWords:
		return upsertEnglishWord, nil
	case Phrases:
		return upsertPhrase, nil
	default:
		return "", fmt.Errorf("upsert %q: invalid dictionary", name)
	}
}

func tableName(name Name) (string, error) {
	switch name {
	case ChineseWords:
		return "cn_words", nil
	case EnglishWords:
		return "en_words", nil
	case Phrases:
		return "phrases", nil
	default:
		return "", fmt.Errorf("delete from %q: invalid dictionary", name)
	}
}
