package dictionary

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Export renders a dictionary in the format expected by Rime.
func (c *Catalog) Export(ctx context.Context, name Name) (Export, error) {
	entries, err := c.List(ctx, name, nil)
	if err != nil {
		return Export{}, fmt.Errorf("export %s: %w", name, err)
	}

	var content strings.Builder
	var filename string
	switch name {
	case ChineseWords:
		filename = "common.dict.yaml"
		writeRimeHeader(&content, "common")
		for _, entry := range entries {
			content.WriteString(entry.Word)
			content.WriteByte('\t')
			content.WriteString(entry.Reading)
			content.WriteByte('\t')
			content.WriteString(strconv.Itoa(entry.Weight))
			content.WriteByte('\n')
		}
	case EnglishWords:
		filename = "common_en.dict.yaml"
		writeRimeHeader(&content, "common_en")
		for _, entry := range entries {
			content.WriteString(entry.Word)
			content.WriteByte('\t')
			content.WriteString(entry.Reading)
			content.WriteByte('\n')
		}
	case Phrases:
		filename = "custom_phrase.txt"
		for _, entry := range entries {
			content.WriteString(entry.Word)
			content.WriteByte('\t')
			content.WriteString(entry.Abbr)
			content.WriteByte('\n')
		}
	default:
		return Export{}, fmt.Errorf("export %q: invalid dictionary", name)
	}
	return Export{Filename: filename, Content: content.String()}, nil
}

func writeRimeHeader(content *strings.Builder, name string) {
	content.WriteString("# Rime dictionary\n# encoding: utf-8\n---\nname: ")
	content.WriteString(name)
	content.WriteString("\nversion: \"")
	content.WriteString(strconv.FormatInt(time.Now().Unix(), 10))
	content.WriteString("\"\nsort: by_weight\n...\n")
}
