package main

import (
	"database/sql"
	"dict-manager/model"
	"dict-manager/store"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

const currentSchemaVer = 100

func initDB(db *sql.DB) error {
	// 1. 创建架构信息表
	schemaMeta := `CREATE TABLE IF NOT EXISTS schema_meta (
    		id INTEGER PRIMARY KEY AUTOINCREMENT,
    		name TEXT NOT NULL UNIQUE,
    		value INTEGER NOT NULL,
    		applied_at INTEGER DEFAULT (unixepoch())
    );`
	_, err := db.Exec(schemaMeta)
	if err != nil {
		return err
	}
	// 2. 读取 schema_ver
	readSchemaVer := `SELECT value FROM schema_meta WHERE name = 'schema_ver';`
	var schemaVerInDB int
	err = db.QueryRow(readSchemaVer).Scan(&schemaVerInDB)
	if errors.Is(err, sql.ErrNoRows) {
		// 无架构信息，创建架构
		err := createSchema(db, currentSchemaVer)
		if err != nil {
			return err
		}
	} else {
		if err != nil {
			return err
		}
		// 表有架构信息，更新架构。
		err := updateSchema(db, schemaVerInDB, currentSchemaVer)
		if err != nil {
			return err
		}
	}
	return nil
}

func createSchema(db *sql.DB, currentVer int) error {
	tx, txErr := db.Begin()
	if txErr != nil {
		return txErr
	}
	defer func(tx *sql.Tx) {
		if txErr != nil {
			if err := tx.Rollback(); err != nil {
				slog.Error("Rollback failed: %v", err)
			}
		}
	}(tx)
	// 插入版本信息
	insert := `INSERT INTO schema_meta (name, value) VALUES (?, ?);`
	_, txErr = tx.Exec(insert, "schema_ver", currentVer)
	if txErr != nil {
		return txErr
	}
	// 并创建表
	// 创建 words 表
	createTable := `CREATE TABLE IF NOT EXISTS words (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			word TEXT NOT NULL UNIQUE,
			reading TEXT NOT NULL,
			weight INTEGER NOT NULL DEFAULT 10,
			category INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
	);`
	if _, txErr = tx.Exec(createTable); txErr != nil {
		return txErr
	}
	txErr = tx.Commit()
	return txErr
}

func updateSchema(db *sql.DB, verInDB int, currentVer int) error {
	tx, txErr := db.Begin()
	if txErr != nil {
		return txErr
	}
	defer func(tx *sql.Tx) {
		if txErr != nil {
			if err := tx.Rollback(); err != nil {
				slog.Error("Rollback failed: %v", err)
			}
		}
	}(tx)
	// 版本 schemaVerInDB 更新到 currentSchemaVer
	update := `UPDATE schema_meta SET value = ? WHERE name = ?;`
	_, txErr = tx.Exec(update, currentVer, "schema_ver")
	if txErr != nil {
		return txErr
	}
	// 更新架构的SQL操作，以后可能会有
	txErr = tx.Commit()
	return txErr
}

func getDB(c *gin.Context) *sql.DB {
	v, _ := c.Get("db")
	return v.(*sql.DB)
}

func listWordsHandler(c *gin.Context) {
	db := getDB(c)
	selectWord := `SELECT id, word, reading, weight, created_at, updated_at
			FROM words
			ORDER BY weight DESC, id;`
	rows, err := db.Query(selectWord)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.Error("Rows close failed: %v", err)
		}
	}(rows)
	var res []model.CnWord
	for rows.Next() {
		var w model.CnWord
		var created sql.NullString
		if err := rows.Scan(&w.ID, &w.Word, &w.Reading, &w.Weight, &created); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if created.Valid {
			if t, err := time.Parse(time.RFC3339, created.String); err == nil {
				w.CreatedAt = t
			}
		}
		res = append(res, w)
	}
	c.JSON(http.StatusOK, res)
}

func addWordsHandler(c *gin.Context) {
	dictName := c.Param("dict_name")
	dictName = model.GetDictName(dictName)
	if dictName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	var request []model.WordItem
	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var errorWords []string
	var err error
	if dictName == "cn_words" {
		errorWords, err = store.UpsertCnWords(getDB(c), request)
	} else if dictName == "en_words" {
		errorWords, err = store.UpsertEnWords(getDB(c), request)
	} else if dictName == "phrases" {
		errorWords, err = store.UpsertPhrases(getDB(c), request)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_words": errorWords})
	return
}

func deleteWordHandler(c *gin.Context) {
	db := getDB(c)
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	_, err = db.Exec("DELETE FROM words WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func exportHandler(c *gin.Context) {
	db := getDB(c)
	fmtType := c.Query("fmt")
	rows, err := db.Query("SELECT word,reading,weight FROM words ORDER BY weight DESC, id ASC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	lines := ""
	for rows.Next() {
		var word string
		var reading sql.NullString
		var weight int
		if err := rows.Scan(&word, &reading, &weight); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if fmtType == "rime" || fmtType == "" {
			// Rime often expects: word\tweight
			lines += fmt.Sprintf("%s\t%d\n", word, weight)
		} else if fmtType == "tsv" {
			// include reading if present
			if reading.Valid && reading.String != "" {
				lines += fmt.Sprintf("%s\t%s\t%d\n", word, reading.String, weight)
			} else {
				lines += fmt.Sprintf("%s\t%d\n", word, weight)
			}
		} else {
			// default fallback
			lines += fmt.Sprintf("%s\t%d\n", word, weight)
		}
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=personal_dict.txt")
	c.String(http.StatusOK, lines)
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

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	api := router.Group("/dicts")
	{
		api.GET("/words", listWordsHandler)
		api.GET("/export", exportHandler)
		api.POST("/dict/:dict_name", addWordsHandler)
		api.DELETE("/words/:id", deleteWordHandler)
	}

	slog.Info("starting server on :8080")
	if err := router.Run(); err != nil {
		slog.Error("start server error", "msg", err)
	}
}
