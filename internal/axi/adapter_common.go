package axi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const maxSafeInteger = 9007199254740991

// DTO validation aborts within an adapter boundary. Only domain errors are
// recovered; programming errors remain visible to Go's runtime and tests.
func adapterRecover(err *error) {
	if value := recover(); value != nil {
		if e, ok := value.(*DomainError); ok {
			*err = e
		} else {
			panic(value)
		}
	}
}
func adapterInvalid(message string)              { panic(NewError("UPSTREAM_SCHEMA_MISMATCH", message, 1)) }
func adapterFail(code, message string, exit int) { panic(NewError(code, message, exit)) }
func adapterObject(value any) Object {
	v, ok := value.(map[string]any)
	if !ok || v == nil {
		adapterInvalid("Run detail does not match the supported DTO contract")
	}
	return v
}
func adapterArray(value any) []any {
	switch v := value.(type) {
	case []any:
		return v
	case []Object:
		a := make([]any, len(v))
		for i, x := range v {
			a[i] = x
		}
		return a
	}
	adapterInvalid("Invalid bounded collection")
	return nil
}
func adapterNumber(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		if v >= -maxSafeInteger && v <= maxSafeInteger {
			return int64(v), true
		}
	case int64:
		if v >= -maxSafeInteger && v <= maxSafeInteger {
			return v, true
		}
	case float64:
		if !math.IsNaN(v) && !math.IsInf(v, 0) && v == math.Trunc(v) && v >= -maxSafeInteger && v <= maxSafeInteger {
			return int64(v), true
		}
	case json.Number:
		approximation, err := strconv.ParseFloat(string(v), 64)
		if err != nil || math.IsNaN(approximation) || math.IsInf(approximation, 0) || approximation != math.Trunc(approximation) || approximation < -maxSafeInteger || approximation > maxSafeInteger {
			return 0, false
		}
		if approximation == 0 {
			mantissa := strings.Split(strings.ToLower(string(v)), "e")[0]
			if strings.Trim(mantissa, "-+.0") == "" {
				return 0, true
			}
			return 0, false
		}
		n, ok := new(big.Rat).SetString(string(v))
		if ok && n.IsInt() && n.Num().IsInt64() {
			value := n.Num().Int64()
			if value >= -maxSafeInteger && value <= maxSafeInteger {
				return value, true
			}
		}
	}
	return 0, false
}
func adapterString(value any) string {
	s, ok := adapterText(value)
	if !ok {
		adapterInvalid("Invalid independent evidence response")
	}
	return s
}
func adapterOptionalString(o Object, key string) any {
	v, present := o[key]
	if !present {
		return nil
	}
	return adapterString(v)
}
func adapterFlag(value any) any {
	if value == nil {
		return nil
	}
	b, ok := value.(bool)
	if !ok {
		adapterInvalid("Invalid boolean metadata")
	}
	return b
}
func utf16Length(s string) int { return len(utf16.Encode([]rune(s))) }
func safeIdentityText(s string, limit int, extended bool) bool {
	if s == "" || !utf8.ValidString(s) || utf16Length(s) > limit {
		return false
	}
	for _, r := range s {
		if r <= 31 || r == 127 || (extended && (r <= 159 && r >= 127 || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069)) {
			return false
		}
	}
	return true
}

var exactPositive = regexp.MustCompile(`^[1-9][0-9]*$`)
var exactPool = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

func adapterIdentity(value any, numeric bool) string {
	var s string
	if v, ok := value.(string); ok {
		s = v
	} else {
		n, ok := adapterNumber(value)
		if !ok || n <= 0 {
			adapterInvalid("Unsafe upstream numeric identity")
		}
		s = strconv.FormatInt(n, 10)
	}
	if !safeIdentityText(s, 256, true) {
		adapterInvalid("Missing or invalid upstream identity")
	}
	if numeric {
		n, e := strconv.ParseInt(s, 10, 64)
		if !exactPositive.MatchString(s) || e != nil || n > maxSafeInteger {
			adapterInvalid("Unsafe upstream run identity")
		}
	}
	return s
}
func Identity(value any, numeric bool) (result string, err error) {
	defer adapterRecover(&err)
	return adapterIdentity(value, numeric), nil
}
func literal(value string) string {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) {
		adapterFail("USAGE_ERROR", "Invalid locator value", 2)
	}
	for _, r := range value {
		if r <= 31 || r == 127 {
			adapterFail("USAGE_ERROR", "Invalid locator value", 2)
		}
	}
	return "($base64:" + base64.RawURLEncoding.EncodeToString([]byte(value)) + ")"
}
func Literal(value string) (result string, err error) {
	defer adapterRecover(&err)
	return literal(value), nil
}
func idCondition(value string) string { return "(id:" + literal(value) + ")" }
func IDCondition(value string) (result string, err error) {
	defer adapterRecover(&err)
	return idCondition(value), nil
}
func branchCondition(value string) string { return "(name:(value:" + literal(value) + "))" }
func BranchCondition(value string) (result string, err error) {
	defer adapterRecover(&err)
	return branchCondition(value), nil
}
func APIPath(resource string, parts []string, fields string) string {
	return "/app/rest/" + resource + "?locator=" + url.QueryEscape(strings.Join(parts, ",")) + "&fields=" + url.QueryEscape(fields)
}
func boundedQuery(q ReadRequest, message string) {
	if q.Count < 1 || q.Count > 100 || q.ScanLimit < 1 || q.ScanLimit > 5000 || q.Start < 0 || q.Start >= q.ScanLimit {
		adapterFail("USAGE_ERROR", message, 2)
	}
}

type AdapterRequest struct {
	Resource, Fields, Path string
	Filters                []string
}

func pageRequest(resource string, filters []string, fields string, q ReadRequest) AdapterRequest {
	parts := append(append([]string{}, filters...), fmt.Sprintf("count:%d", q.Count), fmt.Sprintf("start:%d", q.Start), fmt.Sprintf("lookupLimit:%d", q.ScanLimit))
	return AdapterRequest{resource, fields, APIPath(resource, parts, fields), filters}
}

type ContinuationRequest struct {
	ServerURL, Resource, Fields string
	Filters                     []string
	Count, Start, ScanLimit     int
}

func continuationRequest(r AdapterRequest, q ReadRequest, server string) ContinuationRequest {
	return ContinuationRequest{server, r.Resource, r.Fields, r.Filters, q.Count, q.Start, q.ScanLimit}
}
func continuationInvalid() { adapterInvalid("Provider continuation cannot be safely reconstructed") }

var dimensionKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*:`)
var plainDigits = regexp.MustCompile(`^[0-9]+$`)

func dimensions(locator string) []string {
	if len(locator) > 16384 {
		continuationInvalid()
	}
	depth, start := 0, 0
	parts := []string{}
	for i, r := range locator {
		switch r {
		case '(':
			depth++
			if depth > 8 {
				continuationInvalid()
			}
		case ')':
			depth--
			if depth < 0 {
				continuationInvalid()
			}
		case ',':
			if depth == 0 {
				parts = append(parts, locator[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		continuationInvalid()
	}
	parts = append(parts, locator[start:])
	for _, p := range parts {
		if !dimensionKey.MatchString(p) {
			continuationInvalid()
		}
	}
	return parts
}
func NextPosition(href string, r ContinuationRequest) (result int, err error) {
	defer adapterRecover(&err)
	return nextPosition(href, r), nil
}
func nextPosition(href string, r ContinuationRequest) int {
	if href == "" || len(href) > 16384 || strings.HasPrefix(href, "//") {
		continuationInvalid()
	}
	for _, c := range href {
		if c <= 32 || c == 127 || c == '\\' || c == '#' {
			continuationInvalid()
		}
	}
	rawPath := strings.SplitN(href, "?", 2)[0]
	lower := strings.ToLower(rawPath)
	for _, encoded := range []string{"%2e", "%2f", "%5c", "%25"} {
		if strings.Contains(lower, encoded) {
			continuationInvalid()
		}
	}
	for _, p := range strings.Split(rawPath, "/") {
		if p == "." || p == ".." {
			continuationInvalid()
		}
	}
	base, e := url.Parse(r.ServerURL)
	if e != nil {
		continuationInvalid()
	}
	relative, e := url.Parse(href)
	if e != nil {
		continuationInvalid()
	}
	u := base.ResolveReference(relative)
	origin := func(u *url.URL) string {
		host := strings.ToLower(u.Host)
		if u.Scheme == "https" {
			host = strings.TrimSuffix(host, ":443")
		}
		if u.Scheme == "http" {
			host = strings.TrimSuffix(host, ":80")
		}
		return strings.ToLower(u.Scheme) + "://" + host
	}
	if origin(u) != origin(base) || u.User != nil {
		continuationInvalid()
	}
	prefix := strings.TrimSuffix(base.Path, "/")
	if relative.IsAbs() && !strings.HasPrefix(u.Path, prefix+"/app/rest/") {
		continuationInvalid()
	}
	path := u.Path
	if strings.HasPrefix(path, prefix+"/app/rest/") {
		path = strings.TrimPrefix(path, prefix)
	}
	if path != "/app/rest/"+r.Resource {
		continuationInvalid()
	}
	params, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		continuationInvalid()
	}
	for key := range params {
		if key != "locator" && key != "fields" {
			continuationInvalid()
		}
	}
	if len(params["locator"]) != 1 || len(params["fields"]) > 1 {
		continuationInvalid()
	}
	if len(params["fields"]) == 1 && params.Get("fields") != r.Fields {
		continuationInvalid()
	}
	page := map[string]int{}
	filters := []string{}
	for _, part := range dimensions(params.Get("locator")) {
		k, v, _ := strings.Cut(part, ":")
		if k == "count" || k == "start" || k == "lookupLimit" {
			n, e := strconv.ParseInt(v, 10, 64)
			_, exists := page[k]
			if !plainDigits.MatchString(v) || e != nil || n > maxSafeInteger || exists {
				continuationInvalid()
			}
			page[k] = int(n)
		} else {
			filters = append(filters, part)
		}
	}
	expected := append([]string{}, r.Filters...)
	sort.Strings(expected)
	sort.Strings(filters)
	if strings.Join(expected, "\x00") != strings.Join(filters, "\x00") {
		continuationInvalid()
	}
	start, exists := page["start"]
	lookup, hasLookup := page["lookupLimit"]
	if !hasLookup {
		lookup = r.ScanLimit
	}
	if page["count"] != r.Count || lookup != r.ScanLimit || !exists || start <= r.Start || start >= r.ScanLimit {
		continuationInvalid()
	}
	return start
}
func adapterRows(dto Object, key string, count int) []any {
	rows := adapterArray(dto[key])
	n, ok := adapterNumber(dto["count"])
	if !ok || int(n) != len(rows) || len(rows) > count {
		adapterInvalid("Invalid bounded collection")
	}
	return rows
}
func adapterNote(notes *[]Limitation, code, message, source, runID string) {
	*notes = append(*notes, Limitation{Code: code, Message: message, Source: source, RunID: runID})
}
func uniqueNote(notes *[]Limitation, note Limitation) {
	for _, v := range *notes {
		if v.Code == note.Code {
			return
		}
	}
	*notes = append(*notes, note)
}
func adapterPage(items []Object, returned int, notes []Limitation) Object {
	return Object{"items": items, "providerReturned": returned, "position": nil, "hasMore": nil, "limitations": notes}
}
func adapterContinuation(dto Object, page Object, r AdapterRequest, q ReadRequest, server, source, unsafe, unknown string) {
	notes := page["limitations"].([]Limitation)
	if v, present := dto["nextHref"]; present {
		s, ok := v.(string)
		if ok {
			position, e := NextPosition(s, continuationRequest(r, q, server))
			if e == nil {
				page["position"] = position
				page["hasMore"] = true
				return
			}
		}
		adapterNote(&notes, "UNSAFE_CONTINUATION", unsafe, source, "")
	} else {
		adapterNote(&notes, "SCAN_COVERAGE_UNKNOWN", unknown, source, "")
	}
	page["limitations"] = notes
}
