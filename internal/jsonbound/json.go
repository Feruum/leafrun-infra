// Package jsonbound rejects ambiguous provider JSON before typed decoding.
package jsonbound

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

func Decode(data []byte, out any) error {
	invalid := errors.New("provider JSON invalid")
	if len(data) > 65536 || !utf8.Valid(data) {
		return invalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return invalid
		}
		tok, err := d.Token()
		if err != nil {
			return invalid
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				// encoding/json matches struct fields using Unicode simple folding.
				// Reject aliases before typed decoding can let a later member shadow one.
				folded := strings.Map(func(r rune) rune {
					smallest := r
					for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
						if next < smallest {
							smallest = next
						}
					}
					return smallest
				}, name)
				if err != nil || !ok || seen[folded] {
					return invalid
				}
				seen[folded] = true
				if value(depth+1) != nil {
					return invalid
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return invalid
			}
		case '[':
			for d.More() {
				if value(depth+1) != nil {
					return invalid
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return invalid
			}
		default:
			return invalid
		}
		return nil
	}
	if value(0) != nil {
		return invalid
	}
	if _, err := d.Token(); err != io.EOF {
		return invalid
	}
	if json.Unmarshal(data, out) != nil {
		return invalid
	}
	return nil
}
