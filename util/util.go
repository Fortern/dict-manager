package util

import (
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"
)

func CheckCnWord(word string, reading string) bool {
	// 如果有不是中文的字符，返回false
	for _, c := range word {
		if !unicode.Is(unicode.Scripts["Han"], c) {
			slog.Error("CheckCnWord: word %s is not han", "word", word)
			return false
		}
	}
	split := strings.Split(reading, " ")
	if len(split) != utf8.RuneCountInString(word) {
		slog.Error("CheckCnWord: reading string length mismatch")
		return false
	}
	for _, s := range split {
		for _, c := range s {
			if !unicode.IsLetter(c) {
				slog.Error("CheckCnWord: reading string %s is not letter", "reading", reading)
				return false
			}
		}
	}
	return true
}

func CheckEnWord(word string, reading string) bool {
	for _, c := range word {
		if !unicode.IsLetter(c) && c != ' ' {
			slog.Error("CheckCnWord: word string %s is not letter", "word", word)
			return false
		}
	}
	for _, c := range reading {
		if !unicode.IsLetter(c) && c != ' ' {
			slog.Error("CheckCnWord: reading string %s is not letter", "reading", reading)
			return false
		}
	}
	return true
}

func CheckPhrase(word string, reading string) bool {
	return true
}
