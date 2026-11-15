/*
Purpose: Compare restart specs without losing unknown JSON fields or spilling literals.
Used by: Restart preparation and response verification.
Responsibilities: Normalize object order and equivalent decimal number tokens before hashing.
Important: Only the resulting digest is retained with a pending review.
*/
package operation

import (
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"
)

func specHash(spec map[string]any) ([32]byte, error) {
	if _, err := normalizeSpec(spec); err != nil {
		return [32]byte{}, err
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return [32]byte{}, invalidRestart()
	}
	return sha256.Sum256(encoded), nil
}

func normalizeSpec(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		for key, entry := range v {
			normalized, err := normalizeSpec(entry)
			if err != nil {
				return nil, err
			}
			v[key] = normalized
		}
	case []any:
		for i, entry := range v {
			normalized, err := normalizeSpec(entry)
			if err != nil {
				return nil, err
			}
			v[i] = normalized
		}
	case json.Number:
		return canonicalDecimal(v)
	}
	return value, nil
}

func canonicalDecimal(number json.Number) (json.Number, error) {
	text := string(number)
	exponent := int64(0)
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		var err error
		exponent, err = strconv.ParseInt(text[i+1:], 10, 32)
		if err != nil {
			return "", invalidRestart()
		}
		text = text[:i]
	}
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign, text = "-", text[1:]
	}
	if i := strings.IndexByte(text, '.'); i >= 0 {
		exponent -= int64(len(text) - i - 1)
		text = text[:i] + text[i+1:]
	}
	text = strings.TrimLeft(text, "0")
	if text == "" {
		return "0", nil
	}
	significant := strings.TrimRight(text, "0")
	exponent += int64(len(text) - len(significant))
	if exponent == 0 {
		return json.Number(sign + significant), nil
	}
	return json.Number(sign + significant + "e" + strconv.FormatInt(exponent, 10)), nil
}
