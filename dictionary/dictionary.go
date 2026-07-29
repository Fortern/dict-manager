// Package dictionary owns the word dictionaries, their validation rules, and
// their persistence.
package dictionary

import (
	"fmt"
	"maps"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Name identifies one of the dictionaries managed by the application.
type Name string

const (
	ChineseWords Name = "cn_words"
	EnglishWords Name = "en_words"
	Phrases      Name = "phrases"
)

// ParseName validates an external dictionary name.
func ParseName(value string) (Name, error) {
	name := Name(value)
	switch name {
	case ChineseWords, EnglishWords, Phrases:
		return name, nil
	default:
		return "", fmt.Errorf("unknown dictionary %q", value)
	}
}

// Entry is a word or phrase returned by a dictionary.
//
// Reading is used by the Chinese and English dictionaries. Abbr is used by
// the phrase dictionary. Weight is only used by the Chinese dictionary.
type Entry struct {
	ID        int       `json:"id"`
	Word      string    `json:"word"`
	Reading   string    `json:"reading,omitempty"`
	Abbr      string    `json:"abbr,omitempty"`
	Category  int       `json:"category"`
	Weight    int       `json:"weight,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EntryInput is the HTTP representation accepted when adding dictionary
// entries. For phrases, Reading contains the abbreviation.
type EntryInput struct {
	Word     string `json:"word"`
	Reading  string `json:"reading"`
	Weight   int    `json:"weight,omitempty"`
	Category int    `json:"category"`
}

// Export is a generated dictionary file.
type Export struct {
	Filename string
	Content  string
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

// Categories returns a copy of the supported category names.
func Categories() map[int]string {
	// Go does not provide read-only maps.
	return maps.Clone(categories)
}

func validInput(name Name, input EntryInput) bool {
	switch name {
	case ChineseWords:
		return validChineseWord(input.Word, input.Reading)
	case EnglishWords:
		return validEnglishWord(input.Word, input.Reading)
	case Phrases:
		return input.Word != "" && input.Reading != ""
	default:
		return false
	}
}

func validChineseWord(word, reading string) bool {
	if word == "" || reading == "" {
		return false
	}
	for _, char := range word {
		if !unicode.Is(unicode.Han, char) {
			return false
		}
	}

	syllables := strings.Split(reading, " ")
	if len(syllables) != utf8.RuneCountInString(word) {
		return false
	}
	for _, syllable := range syllables {
		for _, char := range syllable {
			if !unicode.IsLetter(char) {
				return false
			}
		}
	}
	return true
}

func validEnglishWord(word, reading string) bool {
	if word == "" || reading == "" {
		return false
	}
	for _, char := range word {
		if !unicode.IsLetter(char) && char != ' ' {
			return false
		}
	}
	for _, char := range reading {
		if !unicode.IsLetter(char) {
			return false
		}
	}
	return true
}
