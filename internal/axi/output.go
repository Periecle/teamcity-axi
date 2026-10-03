package axi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	assets "github.com/Periecle/teamcity-axi"
	"github.com/santhosh-tekuri/jsonschema/v6"
	toon "github.com/toon-format/toon-go"
)

var secretName = regexp.MustCompile(`(?i)token|password|secret|authorization|cookie|api[_-]?key|private[_-]?key`)

func SecretMatchers(patterns []string) ([]*regexp.Regexp, error) {
	r := []*regexp.Regexp{}
	for _, p := range patterns {
		if len(p) > 128 || strings.ContainsAny(p, "()+?{}|\\") || strings.Contains(strings.ReplaceAll(p, ".*", ""), "*") || strings.Count(p, ".*") > 1 {
			return nil, Usage("Unsafe secret-name pattern")
		}
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, Usage("Unsafe secret-name pattern")
		}
		r = append(r, re)
	}
	return r, nil
}
func KnownSecrets(env map[string]string, patterns, additional []string) []string {
	matchers, err := SecretMatchers(patterns)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for n, v := range env {
		if v == "" {
			continue
		}
		sensitive := secretName.MatchString(n) || contains(additional, n)
		for _, p := range matchers {
			sensitive = sensitive || p.MatchString(n)
		}
		if sensitive {
			seen[v] = true
		}
	}
	r := []string{}
	for v := range seen {
		r = append(r, v)
	}
	sort.Slice(r, func(i, j int) bool {
		if len(r[i]) == len(r[j]) {
			return r[i] < r[j]
		}
		return len(r[i]) > len(r[j])
	})
	return r
}

var osc = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")
var csi = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
var privateKey = regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z]+ )?PRIVATE KEY-----.*?-----END (?:[A-Z]+ )?PRIVATE KEY-----`)
var authSecret = regexp.MustCompile(`(?i)\b((?:Bearer|Basic)\s+)[^\s"\\,;]+`)
var inlineSecret = regexp.MustCompile(`(?i)\b(token|password|secret|api[_-]?key)\s*[:=]\s*[^\s"\\,;]+`)
var urlSecret = regexp.MustCompile(`(?i)(https?://)[^/@\s]+:[^/@\s]+@`)

func SanitizeText(text string, secrets []string) string {
	for _, v := range secrets {
		if v != "" {
			text = strings.ReplaceAll(text, v, "[REDACTED]")
		}
	}
	if strings.ContainsRune(text, '\x1b') {
		text = osc.ReplaceAllString(text, "")
		text = csi.ReplaceAllString(text, "")
	}
	// Preserve invalid UTF-8 normalization and the second secret pass: removing
	// controls can join pieces of a credential that the first pass cannot see.
	if !utf8.ValidString(text) || strings.IndexFunc(text, unsafeTextRune) >= 0 {
		var b strings.Builder
		b.Grow(len(text))
		for _, r := range text {
			if r <= 8 || (r >= 11 && r <= 31) || (r >= 127 && r <= 159) {
				continue
			}
			if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				b.WriteRune(r)
			}
		}
		text = b.String()
	}
	for _, v := range secrets {
		if v != "" {
			text = strings.ReplaceAll(text, v, "[REDACTED]")
		}
	}
	// ReplaceAllString allocates even when no replacement is needed. Most public
	// identities and ordinary prose do not match any recognizable secret form.
	if privateKey.MatchString(text) {
		text = privateKey.ReplaceAllString(text, "[REDACTED PRIVATE KEY]")
	}
	if authSecret.MatchString(text) {
		text = authSecret.ReplaceAllString(text, "${1}[REDACTED]")
	}
	if inlineSecret.MatchString(text) {
		text = inlineSecret.ReplaceAllString(text, "${1}=[REDACTED]")
	}
	if urlSecret.MatchString(text) {
		text = urlSecret.ReplaceAllString(text, "${1}[REDACTED]@")
	}
	return text
}

func unsafeTextRune(r rune) bool {
	return r <= 8 || (r >= 11 && r <= 31) || (r >= 127 && r <= 159) ||
		(r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

var protectedKeys = map[string]bool{}

func init() {
	for _, k := range strings.Fields("schemaVersion command status context data error meta next observedAt complete truncated limitations counts code message retryable details reason argv server project job jobs branch revision vcsRootId revisionMatch searchScope since until timestampBasis scanLimit consistency maxBytes maxChildProcesses childProcesses retries graphNodes limits runId source limit observed ceiling concurrency deadline stdoutCaptureBytes stderrCaptureBytes") {
		protectedKeys[k] = true
	}
}
func sanitizeValue(value any, secrets []string, matchers []*regexp.Regexp, depth int, inMeta bool, protect bool) any {
	if depth > 30 {
		return "[INPUT_DEPTH_LIMIT]"
	}
	switch v := value.(type) {
	case string:
		return SanitizeText(v, secrets)
	case []any:
		r := make([]any, len(v))
		for i, x := range v {
			r[i] = sanitizeValue(x, secrets, matchers, depth+1, inMeta, protect)
		}
		return r
	case map[string]any:
		r := Object{}
		for k, x := range v {
			structural := protect && protectedKeys[k] && (depth == 0 || inMeta)
			sensitive := !structural && secretName.MatchString(k)
			for _, p := range matchers {
				sensitive = sensitive || (!structural && p.MatchString(k))
			}
			key := k
			if !protect || !protectedKeys[k] {
				key = SanitizeText(k, secrets)
			}
			if sensitive {
				r[key] = "[REDACTED]"
			} else {
				r[key] = sanitizeValue(x, secrets, matchers, depth+1, inMeta || (depth == 0 && k == "meta"), protect)
			}
		}
		return r
	}
	return value
}
func jsonValue(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var value any
	err = json.Unmarshal(b, &value)
	return value, err
}
func Sanitize(value any, secrets, patterns []string) any {
	m, err := SecretMatchers(patterns)
	if err != nil {
		return "[UNSAFE_SECRET_PATTERN]"
	}
	normalized, err := jsonValue(value)
	if err != nil {
		return "[INVALID_VALUE]"
	}
	return sanitizeValue(normalized, secrets, m, 0, false, false)
}

type offlineLoader struct{}

func (offlineLoader) Load(uri string) (any, error) {
	return nil, fmt.Errorf("schema is not packaged: %s", uri)
}

var schemaOnce sync.Once
var schemaErr error
var compiledSchemas map[string]*jsonschema.Schema
var schemaValues map[string]Object
var schemaCompiler *jsonschema.Compiler
var schemaMu sync.Mutex

func loadSchemas() {
	compiledSchemas = map[string]*jsonschema.Schema{}
	schemaValues = map[string]Object{}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	c.UseLoader(offlineLoader{})
	files, err := assets.Files.ReadDir("schemas")
	if err != nil {
		schemaErr = err
		return
	}
	for _, f := range files {
		b, err := assets.Files.ReadFile("schemas/" + f.Name())
		if err != nil {
			schemaErr = err
			return
		}
		var s Object
		if err = json.Unmarshal(b, &s); err != nil {
			schemaErr = err
			return
		}
		name := strings.TrimSuffix(f.Name(), ".schema.json")
		schemaValues[name] = s
		if err = c.AddResource(Str(s, "$id"), s); err != nil {
			schemaErr = err
			return
		}
	}
	schemaCompiler = c
}
func PackagedSchema(name string) (Object, error) {
	schemaOnce.Do(loadSchemas)
	if schemaErr != nil {
		return nil, schemaErr
	}
	s, ok := schemaValues[name]
	if !ok {
		return nil, Usage("Unknown schema command")
	}
	v, err := jsonValue(s)
	return Obj(v), err
}
func validateSchema(name string, v any) error {
	value, err := jsonValue(v)
	if err != nil {
		return err
	}
	return validateNormalizedSchema(name, value)
}

// Only accepts the detached JSON tree produced by jsonValue. Both envelope
// and payload validation can share it without repeatedly encoding the response.
func validateNormalizedSchema(name string, value any) error {
	schemaOnce.Do(loadSchemas)
	if schemaErr != nil {
		return schemaErr
	}
	source, ok := schemaValues[name]
	if !ok {
		return fmt.Errorf("unknown schema %s", name)
	}
	// A CLI invocation only needs its configuration, envelope and payload contracts.
	// Compilation is serialized because the compiler also caches referenced schemas.
	schemaMu.Lock()
	s := compiledSchemas[name]
	if s == nil {
		var err error
		s, err = schemaCompiler.Compile(Str(source, "$id"))
		if err != nil {
			schemaMu.Unlock()
			return err
		}
		compiledSchemas[name] = s
	}
	schemaMu.Unlock()
	return s.Validate(value)
}
func ValidateConfig(name string, v any) error {
	if err := validateSchema(name, v); err != nil {
		return Usage("Invalid " + name + " configuration")
	}
	return nil
}
func ValidateResponse(v Response) error {
	normalized, err := jsonValue(v)
	if err != nil {
		return NewError("INTERNAL_ERROR", "Normalized output violated its public contract", 1)
	}
	return validateNormalizedResponse(Obj(normalized))
}

func validateNormalizedResponse(v Object) error {
	if err := validateNormalizedSchema("response", v); err != nil {
		return NewError("INTERNAL_ERROR", "Normalized output violated its public contract", 1)
	}
	if Str(v, "status") != "error" && Str(v, "command") != "schema" {
		name := strings.ReplaceAll(Str(v, "command"), ".", "-")
		if Str(v, "command") == "run.failure" {
			name = "failure"
		}
		if err := validateNormalizedSchema(name, v["data"]); err != nil {
			return NewError("INTERNAL_ERROR", "Normalized payload violated its public contract", 1)
		}
	}
	return nil
}

type Rendered struct {
	Document string
	Response Response
}

func serializeResponse(r Response, format string) (string, error) {
	v, err := jsonValue(r)
	if err != nil {
		return "", err
	}
	return serializeNormalizedResponse(v, format)
}

func serializeNormalizedResponse(v any, format string) (string, error) {
	if format == "json" {
		b, err := json.Marshal(v)
		return string(b) + "\n", err
	}
	s, err := toon.MarshalString(v, toon.WithIndent(2))
	return s + "\n", err
}
func Limitations(v any) []Limitation {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	r := []Limitation{}
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return r
}
func Render(input Response, format string, maxBytes int, secrets, patterns []string, effective Object) (Rendered, error) {
	matchers, err := SecretMatchers(patterns)
	if err != nil {
		return Rendered{}, err
	}
	normalized, err := jsonValue(input)
	if err != nil {
		return Rendered{}, err
	}
	safe := sanitizeValue(normalized, secrets, matchers, 0, false, true)
	b, err := json.Marshal(safe)
	if err != nil {
		return Rendered{}, err
	}
	var value Response
	if err = json.Unmarshal(b, &value); err != nil {
		return Rendered{}, err
	}
	value.SchemaVersion = input.SchemaVersion
	value.Command = input.Command
	value.Status = input.Status
	value.Meta["observedAt"] = input.Meta["observedAt"]
	notes := Limitations(value.Meta["limitations"])
	if len(notes) > 90 {
		groups := map[string][]Limitation{}
		keys := []string{}
		for _, note := range notes {
			key := note.Code + "\x00" + note.Source + "\x00" + note.Message + fmt.Sprint(note.RunID != "")
			if _, ok := groups[key]; !ok {
				keys = append(keys, key)
			}
			groups[key] = append(groups[key], note)
		}
		notes = nil
		for _, k := range keys {
			group := groups[k]
			note := group[0]
			if len(group) > 1 && note.RunID != "" {
				seen := map[string]bool{}
				for _, n := range group {
					seen[n.RunID] = true
				}
				note.RunID = ""
				note.Message += fmt.Sprintf(" (%d distinct executions affected)", len(seen))
			}
			notes = append(notes, note)
		}
		value.Meta["limitations"] = notes
	}
	normalized, err = jsonValue(value)
	if err != nil {
		return Rendered{}, err
	}
	if err = validateNormalizedResponse(Obj(normalized)); err != nil {
		return Rendered{}, err
	}
	doc, err := serializeNormalizedResponse(normalized, format)
	if err != nil {
		return Rendered{}, err
	}
	if len(doc) <= maxBytes {
		return Rendered{doc, value}, nil
	}
	if effective != nil {
		limits := Obj(value.Meta["limits"])
		if limits == nil {
			limits = Object{}
		}
		for k, v := range effective {
			limits[k] = v
		}
		limits["maxBytes"] = maxBytes
		value.Meta["limits"] = limits
	}
	value.Next = nil
	doc, err = serializeResponse(value, format)
	if err != nil {
		return Rendered{}, err
	}
	if len(doc) <= maxBytes {
		return Rendered{doc, value}, nil
	}
	if value.Command == "run.failure" && value.Status != "error" {
		changes := Objects(value.Data["changes"])
		selection := Obj(value.Data["selection"])
		if selection != nil && len(changes) > 0 {
			value.Meta["truncated"] = true
			notes = append(Limitations(value.Meta["limitations"]), Limitation{Code: "OPTIONAL_CHANGES_OMITTED", Message: "Optional contextual changes were reduced to preserve required failure evidence", Source: "changes", RunID: Str(Obj(value.Data["run"]), "id")})
			value.Meta["limitations"] = notes
			for len(changes) > 0 {
				changes = changes[:len(changes)-1]
				value.Data["changes"] = changes
				selection["omittedChanges"] = Int(selection, "omittedChanges") + 1
				for _, s := range Objects(value.Data["sources"]) {
					if Str(s, "kind") == "changes" && Str(s, "runId") == Str(Obj(value.Data["run"]), "id") {
						s["returned"] = len(changes)
						s["state"] = "partial"
						s["reasonCode"] = "OUTPUT_LIMIT_EXCEEDED"
					}
				}
				doc, err = serializeResponse(value, format)
				if err != nil {
					return Rendered{}, err
				}
				if len(doc) <= maxBytes {
					if err = ValidateResponse(value); err != nil {
						return Rendered{}, err
					}
					return Rendered{doc, value}, nil
				}
			}
		}
	}
	meta := Object{"observedAt": input.Meta["observedAt"], "complete": false, "truncated": true, "limitations": []Limitation{{Code: "OUTPUT_LIMIT_EXCEEDED", Message: "No partial serialized stream was emitted", Source: "output"}}}
	if c, ok := input.Meta["counts"]; ok {
		meta["counts"] = c
	}
	if l, ok := value.Meta["limits"]; ok {
		meta["limits"] = l
	}
	value = Response{SchemaVersion: "1.0", Command: input.Command, Status: "error", Context: value.Context, Error: &DomainError{Code: "INPUT_LIMIT_EXCEEDED", Message: "Required output exceeds the byte budget. Narrow the query or increase --max-bytes within the configured ceiling.", Retryable: false, Details: Object{"limit": "maxBytes", "ceiling": maxBytes, "observed": len(doc)}}, Meta: meta}
	doc, err = serializeResponse(value, format)
	if err != nil {
		return Rendered{}, err
	}
	if len(doc) > maxBytes && value.Context != nil {
		for k := range value.Context {
			if !contains([]string{"server", "project", "job", "vcsRootId"}, k) {
				delete(value.Context, k)
			}
		}
		doc, err = serializeResponse(value, format)
	}
	if err != nil {
		return Rendered{}, err
	}
	if len(doc) > maxBytes {
		value.Error.Message = "Output exceeds byte budget"
		delete(value.Meta, "limitations")
		doc, err = serializeResponse(value, format)
	}
	if err != nil {
		return Rendered{}, err
	}
	if len(doc) > maxBytes && value.Context != nil {
		value.Context = Object{"server": value.Context["server"]}
		doc, err = serializeResponse(value, format)
	}
	if err != nil {
		return Rendered{}, err
	}
	if len(doc) > maxBytes {
		return Rendered{}, fmt.Errorf("output budget cannot hold minimal envelope")
	}
	if err = ValidateResponse(value); err != nil {
		return Rendered{}, err
	}
	return Rendered{doc, value}, nil
}
