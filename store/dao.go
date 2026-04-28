package store

import (
	"database/sql"
	"dict-manager/model"
	"dict-manager/util"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const currentSchemaVer = 100

// 创建架构信息表
const createSchemaMetaTable = `
	CREATE TABLE IF NOT EXISTS schema_meta (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		value INTEGER NOT NULL,
		applied_at INTEGER DEFAULT (unixepoch())
	);
`

// select schema_ver
const selectSchemaVer = `SELECT value FROM schema_meta WHERE name = 'schema_ver';`

// insert schema_ver
const insertMeta = `INSERT INTO schema_meta (name, value) VALUES (?, ?);`

// update schema_ver
const updateMeta = `UPDATE schema_meta SET value = ? WHERE name = ?;`

// 创建词库表
const createTables = `
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

const upsertCnWord = `
	INSERT INTO cn_words(word, reading, weight, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(word) DO UPDATE
		SET reading=excluded.reading, weight=excluded.weight, category=excluded.category, updated_at=excluded.updated_at;
`

const upsertEnWord = `
	INSERT INTO en_words(word, reading, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(word) DO UPDATE
		SET reading=excluded.reading, category=excluded.category, updated_at=excluded.updated_at;
`

const upsertPhrase = `
	INSERT INTO phrases(word, reading, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(word) DO UPDATE
		SET reading=excluded.reading, category=excluded.category, updated_at=excluded.updated_at;
`

const selectEnWordsByCategory = `
	SELECT id, word, reading, category, created_at, updated_at
		FROM en_words
		WHERE category = ?;
`

const selectPhrasesByCategory = `
	SELECT id, word, reading, category, created_at, updated_at
		FROM phrases
		WHERE category = ?;
`

const deleteFromCnWords = `DELETE FROM cn_words WHERE id = ?;`

const deleteFromEnWords = `DELETE FROM en_words WHERE id = ?;`

const deleteFromPhrases = `DELETE FROM phrases WHERE id = ?;`

func InitSchema(db *sql.DB) error {
	// 1. 创建架构信息表
	txErr := withTx(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(createSchemaMetaTable)
		return err
	})
	if txErr != nil {
		return txErr
	}
	// 2. 读取词库表
	txErr = withTx(db, func(tx *sql.Tx) error {
		var schemaVerInDB int
		err := tx.QueryRow(selectSchemaVer).Scan(&schemaVerInDB)
		if errors.Is(err, sql.ErrNoRows) {
			// 无架构信息
			// 插入版本信息
			_, e := tx.Exec(insertMeta, "schema_ver", currentSchemaVer)
			if e != nil {
				return e
			}
			// 创建 words 表
			if _, txErr = tx.Exec(createTables); txErr != nil {
				return txErr
			}
		} else {
			if err != nil {
				return err
			}
			// 表有架构信息，更新架构。
			_, e := tx.Exec(updateMeta, currentSchemaVer, "schema_ver")
			if e != nil {
				return e
			}
			// 更新架构的SQL操作，以后可能会有
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	return nil
}

func UpsertCnWords(db *sql.DB, words []model.WordItem) ([]string, error) {
	errWords := make([]string, 0)
	err := withTx(db, func(tx *sql.Tx) error {
		statement, txErr := tx.Prepare(upsertCnWord)
		if txErr != nil {
			return txErr
		}
		defer func(statement *sql.Stmt) {
			if txErr != nil {
				if err := statement.Close(); err != nil {
					slog.Error("Statement.Close failed.", "msg", err)
				}
			}
		}(statement)
		now := time.Now().Unix()
		for _, word := range words {
			text := strings.Trim(word.Word, " ")
			reading := strings.Trim(word.Reading, " ")
			if !util.CheckCnWord(text, reading) {
				errWords = append(errWords, text)
				continue
			}
			if word.Weight <= 0 {
				word.Weight = 10
			}
			// 没有批处理？
			if _, err := statement.Exec(text, reading, word.Weight, word.Category, now, now); err != nil {
				slog.Error("Exec failed.", "msg", err)
			}
		}
		return nil
	})
	return errWords, err
}

func UpsertEnWords(db *sql.DB, words []model.WordItem) ([]string, error) {
	errWords := make([]string, 0)
	err := withTx(db, func(tx *sql.Tx) error {
		statement, txErr := tx.Prepare(upsertEnWord)
		if txErr != nil {
			return txErr
		}
		defer func(statement *sql.Stmt) {
			if txErr != nil {
				if err := statement.Close(); err != nil {
					slog.Error("Statement.Close failed.", "msg", err)
				}
			}
		}(statement)
		now := time.Now().Unix()
		for _, word := range words {
			text := strings.Trim(word.Word, " ")
			reading := strings.Trim(word.Reading, " ")
			if !util.CheckEnWord(text, reading) {
				errWords = append(errWords, text)
				continue
			}
			if _, err := statement.Exec(text, reading, word.Category, now, now); err != nil {
				slog.Error("Exec failed.", "msg", err)
			}
		}
		return nil
	})
	return errWords, err
}

func UpsertPhrases(db *sql.DB, words []model.WordItem) ([]string, error) {
	errWords := make([]string, 0)
	err := withTx(db, func(tx *sql.Tx) error {
		statement, txErr := tx.Prepare(upsertPhrase)
		if txErr != nil {
			return txErr
		}
		defer func(statement *sql.Stmt) {
			if txErr != nil {
				if err := statement.Close(); err != nil {
					slog.Error("Statement.Close failed.", "msg", err)
				}
			}
		}(statement)
		now := time.Now().Unix()
		for _, word := range words {
			text := strings.Trim(word.Word, " ")
			reading := strings.Trim(word.Reading, " ")
			if !util.CheckPhrase(text, reading) {
				errWords = append(errWords, text)
				continue
			}
			if _, err := statement.Exec(text, reading, word.Category, now, now); err != nil {
				slog.Error("Exec failed.", "msg", err)
			}
		}
		return nil
	})
	return errWords, err
}

func GetCnWords(db *sql.DB, categories []int) ([]model.CnWord, error) {
	cnWords := make([]model.CnWord, 0)
	err := withTx(db, func(tx *sql.Tx) error {
		selectCnWords := "SELECT id, word, reading, weight, category, created_at, updated_at FROM cn_words"
		if len(categories) > 0 {
			selectCnWords += " WHERE category IN (" + strings.Join(intsToStrings(categories), ",") + ");"
		} else {
			selectCnWords += ";"
		}
		rows, txErr := tx.Query(selectCnWords)
		if txErr != nil {
			return txErr
		}
		for rows.Next() {
			var cnWord model.CnWord
			var createdAt int64
			var updatedAt int64
			if e := rows.Scan(&cnWord.ID, &cnWord.Word, &cnWord.Reading, &cnWord.Weight, &cnWord.Category, &createdAt, &updatedAt); e != nil {
				slog.Error("rows.Scan failed", "msg", e)
				continue
			}
			cnWord.CreatedAt = time.Unix(createdAt, 0)
			cnWord.UpdatedAt = time.Unix(updatedAt, 0)
			cnWords = append(cnWords, cnWord)
		}
		return rows.Err()
	})
	return cnWords, err
}

func GetEnWords(db *sql.DB, categories []int) ([]model.EnWord, error) {
	enWords := make([]model.EnWord, 0)
	err := withTx(db, func(tx *sql.Tx) error {
		selectEnWords := "SELECT id, word, reading, category, created_at, updated_at FROM en_words"
		if len(categories) > 0 {
			selectEnWords += " WHERE category IN (" + strings.Join(intsToStrings(categories), ",") + ");"
		} else {
			selectEnWords += ";"
		}
		rows, txErr := tx.Query(selectEnWords)
		if txErr != nil {
			return txErr
		}
		for rows.Next() {
			var enWord model.EnWord
			var createdAt int64
			var updatedAt int64
			if e := rows.Scan(&enWord.ID, &enWord.Word, &enWord.Reading, &enWord.Category, &createdAt, &updatedAt); e != nil {
				slog.Error("rows.Scan failed", "msg", e)
				continue
			}
			enWord.CreatedAt = time.Unix(createdAt, 0)
			enWord.UpdatedAt = time.Unix(updatedAt, 0)
			enWords = append(enWords, enWord)
		}
		return rows.Err()
	})
	return enWords, err
}

func GetPhrases(db *sql.DB, categories []int) ([]model.Phrase, error) {
	phrases := make([]model.Phrase, 0)
	err := withTx(db, func(tx *sql.Tx) error {
		selectPhrases := "SELECT id, word, reading, category, created_at, updated_at FROM phrases"
		if len(categories) > 0 {
			selectPhrases += " WHERE category IN (" + strings.Join(intsToStrings(categories), ",") + ");"
		} else {
			selectPhrases += ";"
		}
		rows, txErr := tx.Query(selectPhrases)
		if txErr != nil {
			return txErr
		}
		for rows.Next() {
			var phrase model.Phrase
			var createdAt int64
			var updatedAt int64
			if e := rows.Scan(&phrase.ID, &phrase.Word, &phrase.Abbr, &phrase.Category, &createdAt, &updatedAt); e != nil {
				slog.Error("rows.Scan failed", "msg", e)
				continue
			}
			phrase.CreatedAt = time.Unix(createdAt, 0)
			phrase.UpdatedAt = time.Unix(updatedAt, 0)
			phrases = append(phrases, phrase)
		}
		return rows.Err()
	})
	return phrases, err
}

func DeleteFromCnWordsById(db *sql.DB, id int) error {
	_, err := db.Exec(deleteFromCnWords, id)
	return err
}

func DeleteFromEnWordsById(db *sql.DB, id int) error {
	_, err := db.Exec(deleteFromEnWords, id)
	return err
}

func DeleteFromPhrasesById(db *sql.DB, id int) error {
	_, err := db.Exec(deleteFromPhrases, id)
	return err
}

func withTx(db *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, txErr := db.Begin()
	if txErr != nil {
		return fmt.Errorf("begin tx: %w", txErr)
	}

	defer func(tx *sql.Tx) {
		if txErr != nil {
			if e := tx.Rollback(); e != nil {
				slog.Error("Rollback failed.", "msg", e)
			}
		}
	}(tx)

	// 调用用户传入的事务体
	if txErr = fn(tx); txErr != nil {
		return fmt.Errorf("exec tx: %w", txErr)
	}

	// 提交事务
	if txErr = tx.Commit(); txErr != nil {
		return fmt.Errorf("commit tx: %w", txErr)
	}
	return nil
}

func intsToStrings(ints []int) []string {
	strs := make([]string, len(ints))
	for i := range ints {
		strs[i] = strconv.Itoa(ints[i])
	}
	return strs
}
