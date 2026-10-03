package axi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Go's JSON decoder normally replaces lone UTF-16 surrogates. Retain them as
// visibly escaped text while distinguishing them from well-formed identities.
type malformedJSONText string

func adapterText(value any) (string, bool) {
	switch s := value.(type) {
	case string:
		return s, true
	case malformedJSONText:
		return string(s), true
	}
	return "", false
}
func DecodeJSON(raw []byte) (any, error) {
	marker := "AXI_INVALID_UNICODE_"
	for bytes.Contains(raw, []byte(marker)) {
		marker += "_"
	}
	values := map[string]string{}
	var prepared bytes.Buffer
	for i := 0; i < len(raw); {
		if raw[i] != '"' {
			prepared.WriteByte(raw[i])
			i++
			continue
		}
		start := i
		i++
		for i < len(raw) {
			if raw[i] == '\\' {
				i += 2
				continue
			}
			if raw[i] == '"' {
				i++
				break
			}
			i++
		}
		if i > len(raw) {
			i = len(raw)
		}
		literal := raw[start:i]
		var rewritten bytes.Buffer
		invalid := false
		for j := 0; j < len(literal); {
			if literal[j] == '\\' && j+1 < len(literal) {
				if literal[j+1] == 'u' && j+6 <= len(literal) {
					n, e := strconv.ParseUint(string(literal[j+2:j+6]), 16, 16)
					if e == nil && n >= 0xd800 && n <= 0xdfff {
						if n <= 0xdbff && j+12 <= len(literal) && string(literal[j+6:j+8]) == `\u` {
							low, e := strconv.ParseUint(string(literal[j+8:j+12]), 16, 16)
							if e == nil && low >= 0xdc00 && low <= 0xdfff {
								rewritten.Write(literal[j : j+12])
								j += 12
								continue
							}
						}
						invalid = true
						rewritten.WriteByte('\\')
						rewritten.Write(literal[j : j+6])
						j += 6
						continue
					}
				}
				rewritten.Write(literal[j : j+2])
				j += 2
				continue
			}
			rewritten.WriteByte(literal[j])
			j++
		}
		if !invalid {
			prepared.Write(literal)
			continue
		}
		var text string
		if e := json.Unmarshal(rewritten.Bytes(), &text); e != nil {
			return nil, e
		}
		key := fmt.Sprintf("%s%d", marker, len(values))
		values[key] = text
		replacement, _ := json.Marshal(key)
		prepared.Write(replacement)
	}
	decoder := json.NewDecoder(bytes.NewReader(prepared.Bytes()))
	decoder.UseNumber()
	var result any
	if e := decoder.Decode(&result); e != nil {
		return nil, e
	}
	var extra any
	if e := decoder.Decode(&extra); e == nil {
		return nil, fmt.Errorf("multiple JSON documents")
	} else if e.Error() != "EOF" {
		return nil, e
	}
	var restore func(any) any
	restore = func(value any) any {
		switch v := value.(type) {
		case string:
			if text, ok := values[v]; ok {
				return malformedJSONText(text)
			}
		case []any:
			for i, x := range v {
				v[i] = restore(x)
			}
		case map[string]any:
			for k, x := range v {
				if text, ok := values[k]; ok {
					delete(v, k)
					v[text] = restore(x)
				} else {
					v[k] = restore(x)
				}
			}
		}
		return value
	}
	return restore(result), nil
}
