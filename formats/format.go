package formats

import "errors"

// DocumentParser is an interface for parsing documents from different formats
type DocumentParser interface {
	// Parse parses the input data and returns a slice of documents
	Parse(data []byte) ([]map[string]interface{}, error)
}

// ErrUnsupportedFormat is returned when the requested format is not supported
var ErrUnsupportedFormat = errors.New("unsupported format")

// GetParser returns the appropriate parser for the given format
func GetParser(format string) (DocumentParser, error) {
	switch format {
	case "jsoneachrow":
		return &JSONEachRowParser{}, nil
	case "msgpack":
		return &MsgpackParser{}, nil
	default:
		return nil, ErrUnsupportedFormat
	}
}
