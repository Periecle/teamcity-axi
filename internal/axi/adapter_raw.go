package axi

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type RawResponse struct {
	Status     int
	Body       any
	RetryAfter string
}

var rawStatus = regexp.MustCompile(`^HTTP/1\.1 ([1-5][0-9][0-9]) [\x20-\x7e]{0,100}$`)
var rawHeader = regexp.MustCompile("^([!#$%&'*+.^_`|~0-9A-Za-z-]+):[ \\t]*([^\\r\\n\\x00]*)$")
var jsonContentType = regexp.MustCompile(`^application/[a-z0-9.+-]+\+json$`)

func ParseRaw(c Captured) (result RawResponse, err error) {
	defer adapterRecover(&err)
	if !utf8.Valid(c.Stdout) {
		adapterInvalid("Native response is not UTF-8")
	}
	text := string(c.Stdout)
	a, b := strings.Index(text, "\n\n"), strings.Index(text, "\r\n\r\n")
	offset, separator := a, "\n\n"
	if a < 0 || (b >= 0 && b < a) {
		offset = b
		separator = "\r\n\r\n"
	}
	if offset < 0 || offset > 16384 {
		adapterFail("UPSTREAM_FAILURE", "Native raw response has no supported HTTP envelope", 1)
	}
	lines := strings.Split(strings.ReplaceAll(text[:offset], "\r\n", "\n"), "\n")
	match := rawStatus.FindStringSubmatch(lines[0])
	if match == nil || len(lines)-1 > 64 {
		adapterInvalid("Invalid native HTTP preamble")
	}
	headers := map[string]string{}
	for _, line := range lines[1:] {
		m := rawHeader.FindStringSubmatch(line)
		if m == nil || len(line) > 4096 {
			adapterInvalid("Invalid native HTTP headers")
		}
		name := strings.ToLower(m[1])
		if name == "content-type" || name == "retry-after" {
			if _, ok := headers[name]; ok {
				adapterInvalid("Ambiguous native HTTP headers")
			}
			headers[name] = m[2]
		}
	}
	status := 0
	for _, d := range match[1] {
		status = status*10 + int(d-'0')
	}
	retry := headers["retry-after"]
	if status < 200 || status >= 300 {
		code, message := "UPSTREAM_FAILURE", "The server could not perform this read"
		switch status {
		case 401:
			code, message = "AUTH_REQUIRED", "Native read requires valid authentication"
		case 403:
			code, message = "PERMISSION_DENIED", "The server denied this read"
		case 404:
			code, message = "NOT_FOUND", "The requested resource was not found"
		}
		e := NewError(code, message, 1)
		e.Retryable = status == 429 || status >= 500
		e.HTTPStatus = status
		e.RetryAfter = retry
		panic(e)
	}
	if c.ExitCode != 0 || c.Signal != "" {
		adapterFail("UPSTREAM_FAILURE", "Native process did not complete the successful response", 1)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(headers["content-type"], ";")[0]))
	if contentType == "text/html" {
		adapterFail("AUTH_REQUIRED", "Server returned an HTML authentication or proxy page", 1)
	}
	if contentType != "application/json" && !jsonContentType.MatchString(contentType) {
		adapterInvalid("Native read did not return JSON content")
	}
	body, e := DecodeJSON(c.Stdout[offset+len(separator):])
	if e != nil {
		adapterInvalid("Malformed JSON from native read")
	}
	return RawResponse{status, body, retry}, nil
}
