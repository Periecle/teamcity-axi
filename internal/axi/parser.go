package axi

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Flag struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Choices     []string `json:"choices,omitempty"`
	Min         int      `json:"min,omitempty"`
	Max         int      `json:"max,omitempty"`
}

// Keep explicit zero bounds in schema/help discovery without adding bounds to
// boolean or string flags.
func (f Flag) MarshalJSON() ([]byte, error) {
	value := Object{"type": f.Type, "description": f.Description}
	if len(f.Choices) > 0 {
		value["choices"] = f.Choices
	}
	if f.Type == "integer" || f.Type == "duration" {
		value["min"] = f.Min
		value["max"] = f.Max
	}
	return json.Marshal(value)
}

type Descriptor struct {
	Name       string          `json:"name"`
	Summary    string          `json:"summary"`
	Positional string          `json:"positional,omitempty"`
	Flags      map[string]Flag `json:"flags"`
	Fields     []string        `json:"fields,omitempty"`
}
type Parsed struct {
	Descriptor         Descriptor
	Flags              Object
	Positional, Format string
	Home               bool
}

func (p Parsed) String(name string) string { return Str(p.Flags, name) }
func (p Parsed) Bool(name string) bool     { return Bool(p.Flags, name) }
func (p Parsed) Int(name string, fallback int) int {
	if _, ok := p.Flags[name]; !ok {
		return fallback
	}
	return Int(p.Flags, name)
}

//go:embed registry.json
var registryJSON []byte
var registryData = func() struct {
	Registry    []Descriptor    `json:"registry"`
	GlobalFlags map[string]Flag `json:"globalFlags"`
} {
	var r struct {
		Registry    []Descriptor    `json:"registry"`
		GlobalFlags map[string]Flag `json:"globalFlags"`
	}
	if err := json.Unmarshal(registryJSON, &r); err != nil {
		panic(err)
	}
	return r
}()
var Registry = registryData.Registry
var GlobalFlags = registryData.GlobalFlags

func FindDescriptor(name string) (Descriptor, bool) {
	for _, d := range Registry {
		if d.Name == name {
			return d, true
		}
	}
	return Descriptor{}, false
}
func FlagsFor(d Descriptor) map[string]Flag {
	m := map[string]Flag{}
	for k, v := range GlobalFlags {
		m[k] = v
	}
	for k, v := range d.Flags {
		m[k] = v
	}
	return m
}
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

var optionSyntax = regexp.MustCompile(`(?s)^(?:--([a-z][a-z-]*)(?:=(.*))?|-([hvV]))$`)
var integerSyntax = regexp.MustCompile(`^\d+$`)
var durationSyntax = regexp.MustCompile(`^(\d+)(ms|s|m)$`)
var positiveID = regexp.MustCompile(`^[1-9]\d*$`)
var identifierControls = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func Parse(args []string) (Parsed, error) {
	p := Parsed{Flags: Object{}, Format: "toon"}
	if len(args) > 100 {
		return p, Usage("Arguments exceed the input limit")
	}
	for _, a := range args {
		if len(a) > 4096 {
			return p, Usage("Arguments exceed the input limit")
		}
	}
	known := FlagsFor(Descriptor{})
	for _, d := range Registry {
		for n, f := range d.Flags {
			known[n] = f
		}
	}
	words := []string{}
	literal := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if literal || !strings.HasPrefix(a, "-") {
			words = append(words, a)
			continue
		}
		if a == "--" {
			literal = true
			continue
		}
		m := optionSyntax.FindStringSubmatch(a)
		if m == nil {
			return p, Usage("Invalid option syntax")
		}
		name := m[1]
		if name == "" {
			if m[3] == "h" {
				name = "help"
			} else {
				name = "version"
			}
		}
		f, ok := known[name]
		if !ok {
			return p, Usage("Unknown flag")
		}
		if _, ok = p.Flags[name]; ok {
			return p, Usage("Duplicate singleton flag --" + name)
		}
		hasValue := strings.Contains(a, "=")
		if f.Type == "boolean" {
			if hasValue {
				return p, Usage("--" + name + " takes no value")
			}
			p.Flags[name] = true
			continue
		}
		value := m[2]
		if !hasValue {
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return p, Usage("--" + name + " requires a value")
			}
			value = args[i]
		}
		if value == "" {
			return p, Usage("--" + name + " requires a value")
		}
		if f.Type == "string" {
			if len(f.Choices) > 0 && !contains(f.Choices, value) {
				return p, Usage("Invalid --" + name + " value")
			}
			p.Flags[name] = value
		} else {
			n64 := int64(0)
			var err error
			if f.Type == "duration" {
				m := durationSyntax.FindStringSubmatch(value)
				if m == nil {
					return p, Usage("Invalid --" + name + "; expected duration with ms, s or m suffix")
				}
				n64, err = strconv.ParseInt(m[1], 10, 64)
				if n64 > 1800000 {
					err = fmt.Errorf("overflow")
				}
				if m[2] == "s" {
					n64 *= 1000
				} else if m[2] == "m" {
					n64 *= 60000
				}
			} else {
				if !integerSyntax.MatchString(value) {
					return p, Usage("Invalid --" + name + "; expected integer")
				}
				n64, err = strconv.ParseInt(value, 10, 64)
			}
			if err != nil || n64 < int64(f.Min) || n64 > int64(f.Max) {
				return p, Usage("--" + name + " is outside its allowed range")
			}
			p.Flags[name] = int(n64)
		}
	}
	p.Home = len(words) == 0
	name := "status"
	if len(words) > 0 {
		name = words[0]
		words = words[1:]
	}
	if contains([]string{"run", "job", "queue", "agent", "context"}, name) {
		if len(words) == 0 {
			return p, Usage("Missing " + name + " subcommand")
		}
		name += "." + words[0]
		words = words[1:]
	}
	d, ok := FindDescriptor(name)
	if !ok {
		return p, Usage("Unknown command")
	}
	p.Descriptor = d
	valid := FlagsFor(d)
	for n := range p.Flags {
		if _, ok := valid[n]; !ok {
			return p, Usage("--" + n + " is not valid for " + name)
		}
	}
	max := 0
	if d.Positional != "" {
		max = 1
	}
	if len(words) > max {
		return p, Usage("Unexpected positional arguments")
	}
	if max == 1 && len(words) == 0 && !p.Bool("help") {
		return p, Usage("Missing " + d.Positional)
	}
	if len(words) > 0 {
		p.Positional = words[0]
	}
	if d.Positional == "runId" && len(words) > 0 {
		n, err := strconv.ParseUint(p.Positional, 10, 64)
		if !positiveID.MatchString(p.Positional) || err != nil || n > 9007199254740991 {
			return p, Usage("Run ID must be a positive safe integer")
		}
	}
	if identifierControls.MatchString(p.Positional) {
		return p, Usage("Control characters are not valid identifiers")
	}
	if p.Bool("version") {
		return p, Usage("Version must be requested as a standalone invocation")
	}
	if p.Bool("json") && p.String("format") != "" && p.String("format") != "json" {
		return p, Usage("Conflicting --json and --format")
	}
	for _, group := range [][]string{{"branch", "literal-branch", "all-branches"}, {"failed", "muted"}} {
		n := 0
		for _, k := range group {
			if _, ok := p.Flags[k]; ok {
				n++
			}
		}
		if n > 1 {
			return p, Usage("Conflicting flags: " + strings.Join(group, ", "))
		}
	}
	if p.Bool("include-muted") && !p.Bool("failed") {
		return p, Usage("--include-muted requires --failed")
	}
	conflict := func(keys []string) bool {
		for _, k := range keys {
			if _, ok := p.Flags[k]; ok {
				return true
			}
		}
		return false
	}
	if p.String("test") != "" && conflict([]string{"failed", "muted", "include-muted", "limit", "cursor"}) {
		return p, Usage("--test cannot be combined with filters or pagination")
	}
	if p.String("problem") != "" && conflict([]string{"limit", "cursor"}) {
		return p, Usage("--problem cannot be combined with pagination")
	}
	if name == "run.log" && p.Bool("failed") && conflict([]string{"tail", "contains"}) {
		return p, Usage("--failed cannot be combined with --tail or --contains")
	}
	for _, k := range []string{"since", "until"} {
		if p.String(k) != "" {
			if _, err := CanonicalTimestamp(p.String(k)); err != nil {
				return p, err
			}
		}
	}
	if (p.String("since") != "" || p.String("until") != "") && p.String("state") != "" && p.String("state") != "finished" {
		return p, Usage("Finish-time filters require finished executions")
	}
	if p.String("since") != "" && p.String("until") != "" {
		cmp, err := CompareTimestamps(p.String("since"), p.String("until"))
		if err != nil {
			return p, err
		}
		if cmp > 0 {
			return p, Usage("--since must precede --until")
		}
	}
	if p.String("fields") != "" {
		seen := map[string]bool{}
		for _, f := range strings.Split(p.String("fields"), ",") {
			if seen[f] || !contains(d.Fields, f) {
				return p, Usage("Unknown or duplicate public projection field")
			}
			seen[f] = true
		}
	}
	if p.Bool("json") || p.String("format") == "json" {
		p.Format = "json"
	}
	return p, nil
}
func Help(d *Descriptor) string {
	if d == nil {
		s := "teamcity-axi <command> [arguments] [flags]\nRead-only TeamCity observations.\n\n"
		for _, d := range Registry {
			s += "  " + strings.ReplaceAll(d.Name, ".", " ") + "  " + d.Summary + "\n"
		}
		return s + "\nUse <command> --help for valid flags. Default output: TOON.\n"
	}
	s := "teamcity-axi " + strings.ReplaceAll(d.Name, ".", " ")
	if d.Positional != "" {
		s += " <" + d.Positional + ">"
	}
	s += "\n" + d.Summary + "\n\n"
	flags := FlagsFor(*d)
	keys := []string{}
	for n := range flags {
		keys = append(keys, n)
	}
	sort.Strings(keys)
	for _, n := range keys {
		f := flags[n]
		s += "  --" + n
		if f.Type != "boolean" {
			s += " VALUE"
		}
		s += "  " + f.Description + "\n"
	}
	return s
}
