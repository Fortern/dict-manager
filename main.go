package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

type Word struct {
	ID        int       `json:"id"`
	Word      string    `json:"word"`
	Reading   string    `json:"reading"`
	Category  int       `json:"category"`
	Weight    int       `json:"weight"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func initDB(db *sql.DB) error {
	currentSchemaVer := 100
	// 1. 创建架构信息表
	schemaMeta := `CREATE TABLE IF NOT EXISTS schema_meta (
    	id INTEGER PRIMARY KEY AUTOINCREMENT,
    	name TEXT NOT NULL UNIQUE,
    	value INTEGER NOT NULL,
    	applied_at DATETIME DEFAULT (datetime('now'))
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
		if tx != nil && txErr != nil {
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
		weight INTEGER NOT NULL DEFAULT 999,
		created_at DATETIME NOT NULL
	);`
	if _, txErr = tx.Exec(createTable); txErr != nil {
		return txErr
	}
	return tx.Commit()
}

func updateSchema(db *sql.DB, verInDB int, currentVer int) error {
	if verInDB == currentVer {
		return nil
	}
	tx, txErr := db.Begin()
	defer func(tx *sql.Tx) {
		if tx != nil && txErr != nil {
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
	return tx.Commit()
}

func getDB(c *gin.Context) *sql.DB {
	v, _ := c.Get("db")
	return v.(*sql.DB)
}

func listWordsHandler(c *gin.Context) {
	db := getDB(c)
	rows, err := db.Query("SELECT id , word, reading, weight, created_at, updateed_at FROM words ORDER BY weight DESC, id ASC")
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
	var res []Word
	for rows.Next() {
		var w Word
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

func addWordHandler(c *gin.Context) {
	db := getDB(c)
	var req struct {
		Word    string `json:"word" binding:"required"`
		Reading string `json:"reading"`
		Weight  int    `json:"weight"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}
	now := time.Now().Format(time.RFC3339)
	// upsert by word
	stmt := `INSERT INTO words(word,reading,weight,created_at) VALUES(?,?,?,?) ON CONFLICT(word) DO UPDATE SET reading=excluded.reading, weight=excluded.weight`
	res, err := db.Exec(stmt, req.Word, req.Reading, req.Weight, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(http.StatusOK, gin.H{"id": id})
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
		slog.Error("open sqlite db error", "msg", err)
		return
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			slog.Error("Error closing sqlite db.", "msg", err)
		}
	}(db)

	if err := initDB(db); err != nil {
		slog.Error("init db error.", "msg", err)
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

	api := router.Group("/dict")
	{
		api.GET("/words", listWordsHandler)
		api.GET("/export", exportHandler)
		// mutating endpoints require token auth
		api.POST("/words", addWordHandler)
		api.DELETE("/words/:id", deleteWordHandler)
	}

	slog.Info("starting server on :8080")
	if err := router.Run(); err != nil {
		slog.Error("start server error", "msg", err)
	}
}
