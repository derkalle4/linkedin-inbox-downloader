package browser

import (
	"bytes"
	"encoding/json"
)

// SoftString accepts JSON strings and coerces/ignores other shapes
// so a single unexpected object field does not fail the whole decode.
type SoftString string

func (s SoftString) String() string { return string(s) }

func (s *SoftString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	switch b[0] {
	case '"':
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = SoftString(v)
		return nil
	case '{', '[':
		*s = ""
		return nil
	default:
		*s = SoftString(string(b))
		return nil
	}
}

// SoftStrings accepts a JSON array of strings (or skips non-strings).
type SoftStrings []string

func (s *SoftStrings) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = nil
		return nil
	}
	if b[0] != '[' {
		*s = nil
		return nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		*s = nil
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		var ss SoftString
		if err := json.Unmarshal(item, &ss); err != nil || ss == "" {
			continue
		}
		out = append(out, string(ss))
	}
	*s = out
	return nil
}
