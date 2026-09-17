package plugin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// column kinds, decided per column from every non-null value in it.
type kind int

const (
	kindUnknown kind = iota
	kindNumber
	kindBool
	kindTime
	kindString
)

// row is one SWIS result object with its key order preserved. encoding/json's map loses
// the order, and the order is the SELECT list, which is what a table panel should show.
type row struct {
	keys   []string
	values map[string]json.RawMessage
}

func decodeRows(raw json.RawMessage) ([]row, error) {
	var objects []json.RawMessage
	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil, fmt.Errorf("SWIS results are not a JSON array: %w", err)
	}
	rows := make([]row, 0, len(objects))
	for _, obj := range objects {
		r, err := decodeRow(obj)
		if err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func decodeRow(obj json.RawMessage) (row, error) {
	r := row{values: map[string]json.RawMessage{}}
	dec := json.NewDecoder(strings.NewReader(string(obj)))
	tok, err := dec.Token()
	if err != nil {
		return r, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return r, fmt.Errorf("SWIS result row is not a JSON object")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return r, err
		}
		key := keyTok.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return r, err
		}
		if _, dup := r.values[key]; !dup {
			r.keys = append(r.keys, key)
		}
		r.values[key] = val
	}
	return r, nil
}

// SWIS serialises DateTime values without a zone designator, and occasionally with one.
var timeLayouts = []string{
	"2006-01-02T15:04:05.9999999Z07:00",
	"2006-01-02T15:04:05.9999999",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
}

func parseSwisTime(s string) (time.Time, bool) {
	if len(s) < 19 || s[4] != '-' || s[10] != 'T' {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func classify(val json.RawMessage) kind {
	s := strings.TrimSpace(string(val))
	switch {
	case s == "" || s == "null":
		return kindUnknown
	case s == "true" || s == "false":
		return kindBool
	case s[0] == '"':
		var str string
		if json.Unmarshal(val, &str) == nil {
			if _, ok := parseSwisTime(str); ok {
				return kindTime
			}
		}
		return kindString
	case s[0] == '{' || s[0] == '[':
		return kindString
	default:
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return kindNumber
		}
		return kindString
	}
}

// merge decides the column kind from two observations. A column that is sometimes a
// number and sometimes a string is a string column; a column that is only ever null is
// left unknown and rendered as a nullable string.
func merge(a, b kind) kind {
	switch {
	case a == kindUnknown:
		return b
	case b == kindUnknown, a == b:
		return a
	default:
		return kindString
	}
}

// ToFrame turns SWIS rows into one Grafana data frame with typed, nullable fields.
func ToFrame(name string, raw json.RawMessage, maxRows int) (*data.Frame, bool, error) {
	rows, err := decodeRows(raw)
	if err != nil {
		return nil, false, err
	}
	truncated := false
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
		truncated = true
	}

	var columns []string
	seen := map[string]bool{}
	kinds := map[string]kind{}
	for _, r := range rows {
		for _, k := range r.keys {
			if !seen[k] {
				seen[k] = true
				columns = append(columns, k)
			}
			kinds[k] = merge(kinds[k], classify(r.values[k]))
		}
	}

	frame := data.NewFrame(name)
	for _, col := range columns {
		switch kinds[col] {
		case kindNumber:
			vals := make([]*float64, len(rows))
			for i, r := range rows {
				if v, ok := r.values[col]; ok && classify(v) == kindNumber {
					f, _ := strconv.ParseFloat(strings.TrimSpace(string(v)), 64)
					vals[i] = &f
				}
			}
			frame.Fields = append(frame.Fields, data.NewField(col, nil, vals))
		case kindBool:
			vals := make([]*bool, len(rows))
			for i, r := range rows {
				if v, ok := r.values[col]; ok && classify(v) == kindBool {
					b := strings.TrimSpace(string(v)) == "true"
					vals[i] = &b
				}
			}
			frame.Fields = append(frame.Fields, data.NewField(col, nil, vals))
		case kindTime:
			vals := make([]*time.Time, len(rows))
			for i, r := range rows {
				if v, ok := r.values[col]; ok && classify(v) == kindTime {
					var s string
					_ = json.Unmarshal(v, &s)
					t, _ := parseSwisTime(s)
					vals[i] = &t
				}
			}
			frame.Fields = append(frame.Fields, data.NewField(col, nil, vals))
		default:
			vals := make([]*string, len(rows))
			for i, r := range rows {
				v, ok := r.values[col]
				if !ok || strings.TrimSpace(string(v)) == "null" {
					continue
				}
				var s string
				if json.Unmarshal(v, &s) != nil {
					s = string(v) // a nested object or array, shown as its JSON text
				}
				vals[i] = &s
			}
			frame.Fields = append(frame.Fields, data.NewField(col, nil, vals))
		}
	}
	return frame, truncated, nil
}
