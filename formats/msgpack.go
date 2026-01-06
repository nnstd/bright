package formats

import (
	"fmt"

	"github.com/hashicorp/go-msgpack/codec"
)

// MsgpackParser implements DocumentParser for MessagePack format
// Expects an array of maps in MessagePack format
type MsgpackParser struct{}

// Parse parses MessagePack format data
func (p *MsgpackParser) Parse(data []byte) ([]map[string]interface{}, error) {
	var documents []map[string]interface{}

	decoder := codec.NewDecoderBytes(data, &codec.MsgpackHandle{})

	if err := decoder.Decode(&documents); err != nil {
		return nil, fmt.Errorf("invalid MessagePack data: %w", err)
	}

	return documents, nil
}
