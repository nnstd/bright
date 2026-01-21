package postgres

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Mapper converts PostgreSQL rows to document maps
type Mapper struct {
	config *Config
}

// NewMapper creates a new Mapper
func NewMapper(config *Config) *Mapper {
	return &Mapper{config: config}
}

// RowToDocument converts a pgx.Rows row to a document map
func (m *Mapper) RowToDocument(rows pgx.Rows) (map[string]any, error) {
	fieldDescs := rows.FieldDescriptions()
	values, err := rows.Values()
	if err != nil {
		return nil, fmt.Errorf("failed to get row values: %w", err)
	}

	doc := make(map[string]any)
	for i, fd := range fieldDescs {
		colName := string(fd.Name)

		// Skip if we have a column filter and this column isn't in it
		if len(m.config.Columns) > 0 && !contains(m.config.Columns, colName) {
			continue
		}

		// Apply column mapping if configured
		docField := colName
		if mapped, ok := m.config.ColumnMapping[colName]; ok {
			docField = mapped
		}

		// Convert PostgreSQL types to Go types
		doc[docField] = m.convertValue(values[i])
	}

	return doc, nil
}

// convertValue converts PostgreSQL values to JSON-compatible types
func (m *Mapper) convertValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case time.Time:
		return val.Format(time.RFC3339)

	case pgtype.Numeric:
		f, err := val.Float64Value()
		if err != nil || !f.Valid {
			return nil
		}
		return f.Float64

	case pgtype.Int4:
		if !val.Valid {
			return nil
		}
		return val.Int32

	case pgtype.Int8:
		if !val.Valid {
			return nil
		}
		return val.Int64

	case pgtype.Float4:
		if !val.Valid {
			return nil
		}
		return val.Float32

	case pgtype.Float8:
		if !val.Valid {
			return nil
		}
		return val.Float64

	case pgtype.Bool:
		if !val.Valid {
			return nil
		}
		return val.Bool

	case pgtype.Text:
		if !val.Valid {
			return nil
		}
		return val.String

	case pgtype.UUID:
		if !val.Valid {
			return nil
		}
		return fmt.Sprintf("%x-%x-%x-%x-%x",
			val.Bytes[0:4], val.Bytes[4:6], val.Bytes[6:8],
			val.Bytes[8:10], val.Bytes[10:16])

	case pgtype.Timestamp:
		if !val.Valid {
			return nil
		}
		return val.Time.Format(time.RFC3339)

	case pgtype.Timestamptz:
		if !val.Valid {
			return nil
		}
		return val.Time.Format(time.RFC3339)

	case pgtype.Date:
		if !val.Valid {
			return nil
		}
		return val.Time.Format("2006-01-02")

	case []byte:
		return string(val)

	case string:
		return val

	case int, int8, int16, int32, int64:
		return val

	case uint, uint8, uint16, uint32, uint64:
		return val

	case float32, float64:
		return val

	case bool:
		return val

	default:
		// For arrays and other complex types, convert to string
		return fmt.Sprintf("%v", val)
	}
}

// GetPrimaryKeyValue extracts the primary key value from a document
func (m *Mapper) GetPrimaryKeyValue(doc map[string]any) (string, error) {
	pk := m.config.PrimaryKey

	// Check if column mapping applies
	docField := pk
	if mapped, ok := m.config.ColumnMapping[pk]; ok {
		docField = mapped
	}

	val, ok := doc[docField]
	if !ok {
		return "", fmt.Errorf("primary key %s not found in document", docField)
	}

	return fmt.Sprintf("%v", val), nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
