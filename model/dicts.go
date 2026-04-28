package model

import (
	"time"
)

// CnWord 描述中文词表中的Word实体，也用于响应
type CnWord struct {
	ID        int       `json:"id"`
	Word      string    `json:"word"`
	Reading   string    `json:"reading"`
	Category  int       `json:"category"`
	Weight    int       `json:"weight"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EnWord 描述英文词表中的Word实体，也用于响应
type EnWord struct {
	ID        int       `json:"id"`
	Word      string    `json:"word"`
	Reading   string    `json:"reading"`
	Category  int       `json:"category"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Phrase 描述短语词表的Word实体，也用于响应
type Phrase struct {
	ID        int       `json:"id"`
	Word      string    `json:"word"`
	Abbr      string    `json:"abbr"`
	Category  int       `json:"category"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WordItem http请求中的word实体
type WordItem struct {
	Word     string `json:"word" binding:"required"`
	Reading  string `json:"reading" binding:"required"`
	Weight   int    `json:"weight,omitempty"`
	Category int    `json:"category" binding:"required"`
}

var categories = map[int]string{
	1: "Name",
	2: "Amusement",
	3: "Internet",
	4: "Development",
	5: "Information",
	6: "Medicine",
	7: "Game",
	8: "Minecraft",
	9: "Hollow Knight",
}

var dicts = map[string]string{
	"cn_words": "cn_words",
	"en_words": "en_words",
	"phrases":  "phrases",
}

func GetDictName(name string) string {
	dict, ok := dicts[name]
	if ok {
		return dict
	}
	return ""
}

func GetCategories() map[int]string {
	// 都怪 Go 不提供只读 Map
	m := make(map[int]string, len(categories))
	for k, v := range categories {
		m[k] = v
	}
	return m
}
