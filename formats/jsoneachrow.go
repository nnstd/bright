package formats

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

// JSONEachRowParser implements DocumentParser for JSON Lines format
// Each line is a separate JSON object
type JSONEachRowParser struct{}

// Parse parses JSON Lines format (one JSON object per line)
func (p *JSONEachRowParser) Parse(data []byte) ([]map[string]any, error) {
	var documents []map[string]any

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		var doc map[string]any
		if err := sonic.UnmarshalString(line, &doc); err != nil {
			return nil, fmt.Errorf("invalid JSON on line %d: %w", lineNum, err)
		}

		documents = append(documents, doc)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	return documents, nil
}
