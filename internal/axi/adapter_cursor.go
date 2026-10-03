package axi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

type CursorBinding struct {
	Command    string      `json:"command"`
	Server     string      `json:"server"`
	FilterHash string      `json:"filterHash"`
	Count      int         `json:"count"`
	Window     *TimeWindow `json:"window,omitempty"`
}
type Cursor struct {
	CursorBinding
	Version   int   `json:"version"`
	Position  int   `json:"position"`
	ExpiresAt int64 `json:"expiresAt"`
}

var cursorCommand = regexp.MustCompile(`^[a-z]+\.[a-z]+$`)
var cursorHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var cursorToken = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func cursorInvalid() error {
	return Usage("Cursor is invalid, expired, or belongs to a different query")
}
func validCursor(c Cursor, now int64) bool {
	if c.Version != 1 || !cursorCommand.MatchString(c.Command) || c.Server == "" || utf16Length(c.Server) > 256 || !cursorHash.MatchString(c.FilterHash) || c.Count < 1 || c.Count > 100 || c.Position < 1 || c.Position >= 5000 || c.ExpiresAt <= now || c.ExpiresAt > now+1800000 || c.ExpiresAt > maxSafeInteger {
		return false
	}
	if c.Window != nil {
		a, e := CanonicalTimestamp(c.Window.Since)
		if e != nil || a != c.Window.Since {
			return false
		}
		b, e := CanonicalTimestamp(c.Window.Until)
		if e != nil || b != c.Window.Until {
			return false
		}
		n, e := CompareTimestamps(a, b)
		if e != nil || n > 0 {
			return false
		}
	}
	return true
}
func EncodeCursor(c Cursor, now int64) (string, error) {
	if !validCursor(c, now) {
		return "", cursorInvalid()
	}
	bytes, e := json.Marshal(c)
	if e != nil {
		return "", cursorInvalid()
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	if len(token) > 4096 {
		return "", cursorInvalid()
	}
	return token, nil
}
func DecodeCursor(token string, now int64) (Cursor, error) {
	var c Cursor
	if len(token) > 4096 || !cursorToken.MatchString(token) {
		return c, cursorInvalid()
	}
	raw, e := base64.RawURLEncoding.DecodeString(token)
	if e != nil || !utf8.Valid(raw) || base64.RawURLEncoding.EncodeToString(raw) != token {
		return c, cursorInvalid()
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(raw, &shape) != nil {
		return c, cursorInvalid()
	}
	if window, present := shape["window"]; present && bytes.Equal(bytes.TrimSpace(window), []byte("null")) {
		return c, cursorInvalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&c); e != nil || !validCursor(c, now) {
		return Cursor{}, cursorInvalid()
	}
	var trailing any
	if e = decoder.Decode(&trailing); e == nil || e.Error() != "EOF" {
		return Cursor{}, cursorInvalid()
	}
	return c, nil
}
func AssertCursor(c Cursor, b CursorBinding) error {
	if c.Command != b.Command || c.Server != b.Server || c.FilterHash != b.FilterHash || c.Count != b.Count {
		return cursorInvalid()
	}
	if c.Window == nil && b.Window == nil {
		return nil
	}
	if c.Window == nil || b.Window == nil || *c.Window != *b.Window {
		return cursorInvalid()
	}
	return nil
}
