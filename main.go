package main

import (
	"database/sql"
	"dict-manager/model"
	"dict-manager/store"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func getDB(c *gin.Context) *sql.DB {
	v, _ := c.Get("db")
	return v.(*sql.DB)
}

func listWordsHandler(c *gin.Context) {
	dictName := c.Param("dict_name")
	dictName = model.GetDictName(dictName)
	if dictName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	category, err := strconv.Atoi(c.Query("category"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db := getDB(c)
	var errorWords any
	if dictName == "cn_words" {
		errorWords, err = store.GetCnWords(db, []int{category})
	} else if dictName == "en_words" {
		errorWords, err = store.GetEnWords(db, []int{category})
	} else if dictName == "phrases" {
		errorWords, err = store.GetPhrases(db, []int{category})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	if err != nil {
		slog.Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, errorWords)
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
	db := getDB(c)
	if dictName == "cn_words" {
		errorWords, err = store.UpsertCnWords(db, request)
	} else if dictName == "en_words" {
		errorWords, err = store.UpsertEnWords(db, request)
	} else if dictName == "phrases" {
		errorWords, err = store.UpsertPhrases(db, request)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	if err != nil {
		slog.Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_words": errorWords})
	return
}

func deleteWordHandler(c *gin.Context) {
	dictName := c.Param("dict_name")
	dictName = model.GetDictName(dictName)
	if dictName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	id, err := strconv.Atoi(c.PostForm("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db := getDB(c)
	if dictName == "cn_words" {
		err = store.DeleteFromCnWordsById(db, id)
	} else if dictName == "en_words" {
		err = store.DeleteFromEnWordsById(db, id)
	} else if dictName == "phrases" {
		err = store.DeleteFromPhrasesById(db, id)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	if err != nil {
		slog.Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func exportHandler(c *gin.Context) {
	dictName := c.Param("dict_name")
	dictName = model.GetDictName(dictName)
	if dictName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	db := getDB(c)
	var lines string
	var err error
	var fileName string
	if dictName == "cn_words" {
		lines, err = exportCnWords(db)
		fileName = "common.dict.yaml"
	} else if dictName == "en_words" {
		lines, err = exportEnWords(db)
		fileName = "common_en.dict.yaml"
	} else if dictName == "phrases" {
		lines, err = exportPhrases(db)
		fileName = "custom_phrase.txt"
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_param 'dict_name' is invalid"})
		return
	}
	if err != nil {
		slog.Error(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.String(http.StatusOK, lines)
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

func getCategoriesHandler(c *gin.Context) {
	c.JSON(http.StatusOK, model.GetCategories())
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
		// 查询
		api.GET("/dict/:dict_name", listWordsHandler)
		// 添加新词
		api.POST("/dict/:dict_name", addWordsHandler)
		// 删除词语
		api.DELETE("/dict/:dict_name", deleteWordHandler)
		// 导出文件
		api.GET("/export/:dict_name", exportHandler)
		// 类别列表
		api.GET("/category", getCategoriesHandler)
	}

	slog.Info("starting server on :8080")
	if err := router.Run(); err != nil {
		slog.Error("start server error", "msg", err)
	}
}
