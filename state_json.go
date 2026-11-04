package main

import (
	"bytes"
	"encoding/json"
	"io"
)

// encoding/json otherwise silently accepts duplicate object keys (including
// differently escaped spellings), potentially discarding retained state evidence.
func uniqueJSONKeys(content []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(content))
	var value func() bool
	value = func() bool {
		token, err := dec.Token()
		if err != nil {
			return false
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				token, err := dec.Token()
				if err != nil {
					return false
				}
				key, ok := token.(string)
				if !ok || seen[key] {
					return false
				}
				seen[key] = true
				if !value() {
					return false
				}
			}
		case '[':
			for dec.More() {
				if !value() {
					return false
				}
			}
		default:
			return false
		}
		end, err := dec.Token()
		return err == nil && ((delim == '{' && end == json.Delim('}')) || (delim == '[' && end == json.Delim(']')))
	}
	if !value() {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}
