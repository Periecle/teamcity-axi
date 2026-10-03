package axi

import (
	"context"
	"encoding/json"
	"io"
	"strings"
)

const Version = "0.1.1"

func defaultMaxBytes(name string) int {
	switch name {
	case "status":
		return 6144
	case "run.watch":
		return 8192
	case "run.tree", "run.failure":
		return 24576
	case "schema":
		return 65536
	}
	return 16384
}
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && contains([]string{"--version", "-v", "-V"}, args[0]) {
		_, err := io.WriteString(stdout, Version+"\n")
		if err != nil {
			return 1
		}
		return 0
	}
	format := "toon"
	for i, a := range args {
		if a == "--json" || a == "--format=json" || (a == "json" && i > 0 && args[i-1] == "--format") {
			format = "json"
		}
	}
	command := "status"
	maxBytes := 6144
	exit := 0
	var output Response
	var patterns, additional []string
	var env map[string]string
	var safeContext, effective Object
	debug := false
	p, err := Parse(args)
	if err == nil {
		format = p.Format
		command = p.Descriptor.Name
		maxBytes = p.Int("max-bytes", defaultMaxBytes(command))
		debug = p.Bool("debug")
		if p.Bool("help") {
			if format == "json" {
				var v any = p.Descriptor
				if p.Home {
					v = Object{"commands": Registry}
				}
				b, e := json.Marshal(v)
				if e != nil {
					return 1
				}
				_, e = io.WriteString(stdout, string(b)+"\n")
				if e != nil {
					return 1
				}
			} else {
				var d *Descriptor
				if !p.Home {
					d = &p.Descriptor
				}
				if _, e := io.WriteString(stdout, Help(d)); e != nil {
					return 1
				}
			}
			return 0
		}
		if command == "schema" {
			var d Descriptor
			var ok bool
			d, ok = FindDescriptor(p.Positional)
			if !ok {
				err = Usage("Unknown schema command")
			} else {
				var envelope, payload Object
				envelope, err = PackagedSchema("response")
				if err == nil {
					name := strings.ReplaceAll(d.Name, ".", "-")
					if d.Name == "run.failure" {
						name = "failure"
					}
					if d.Name != "schema" {
						payload, err = PackagedSchema(name)
					}
					output = NewResponse(command, Object{"descriptor": d, "envelope": envelope})
					if payload != nil {
						output.Data["payload"] = payload
					}
				}
			}
		} else {
			var ec ExecutionContext
			env = Environment()
			ec, err = ResolveContext(ctx, p, env)
			if err == nil {
				patterns = secretPatterns(ec)
				if ec.Config != nil && ec.Server != "" {
					additional = ec.Config.Servers[ec.Server].ForwardHeaderEnvNames
				}
				if ec.Config != nil && ec.Config.Limits.MaxBytes > 0 {
					maxBytes = min(maxBytes, ec.Config.Limits.MaxBytes)
				}
				effective = PublicLimits(ReadLimits(ec, ReadProfile(command)), maxBytes)
				if ec.Server != "" {
					safeContext = Object{"server": ec.Server}
					for _, n := range []string{"job", "project"} {
						if p.String(n) != "" {
							safeContext[n] = p.String(n)
						}
					}
				}
				if command == "status" && p.Home && len(ec.Jobs) == 0 {
					var dirty any
					if ec.Dirty != nil {
						dirty = *ec.Dirty
					}
					output = NewResponse(command, Object{"mode": "unconfigured", "checkout": Object{"head": nullString(ec.Head), "branch": nullString(ec.Branch), "dirty": dirty}, "message": "Configure a trusted server and repository binding to observe this checkout"})
					output.Context = PublicContext(ec)
					if p.Bool("check") {
						exit = 1
					}
					if !p.Bool("no-hints") {
						output.Next = []Object{{"reason": "Inspect local context", "argv": []string{"teamcity-axi", "context", "show"}}}
					}
				} else if ec.Server == "" && !(command == "context.show" && !p.Bool("verify")) && !(command == "doctor" && p.Bool("offline")) {
					err = NewError("CONTEXT_REQUIRED", "Select a registered trusted server", 2)
				} else {
					output, exit, err = Dispatch(ctx, p, ec)
				}
			}
		}
	}
	if err != nil {
		domain := AsDomainError(err)
		output = Response{SchemaVersion: "1.0", Command: command, Status: "error", Context: safeContext, Error: domain, Meta: Object{"observedAt": ObservedAt(), "complete": false, "truncated": false}}
		exit = domain.ExitCode
		if exit == 0 {
			exit = 1
		}
	}
	limitHit := Bool(output.Meta, "truncated")
	codes := []string{}
	if output.Error != nil {
		codes = append(codes, output.Error.Code)
	}
	for _, n := range Limitations(output.Meta["limitations"]) {
		codes = append(codes, n.Code)
	}
	for _, c := range codes {
		if strings.Contains(c, "LIMIT") || strings.Contains(c, "BUDGET") || strings.Contains(c, "DEADLINE") {
			limitHit = true
		}
	}
	if effective != nil && (debug || limitHit) {
		output.Meta["limits"] = effective
	}
	if env == nil {
		env = Environment()
	}
	secrets := KnownSecrets(env, patterns, additional)
	rendered, e := Render(output, format, maxBytes, secrets, patterns, effective)
	if e != nil {
		fallback := NewResponse(command, nil)
		fallback.Status = "error"
		fallback.Meta["complete"] = false
		fallback.Error = NewError("INTERNAL_ERROR", "Cannot render the normalized result safely", 1)
		rendered, e = Render(fallback, format, max(2048, maxBytes), secrets, patterns, nil)
		exit = 1
	}
	if e != nil {
		return 1
	}
	if rendered.Response.Status == "error" && exit == 0 {
		exit = 1
	}
	if debug {
		limits := rendered.Response.Meta["limits"]
		if limits == nil {
			limits = effective
		}
		if limits == nil {
			limits = Object{"maxBytes": maxBytes}
		}
		b, e := json.Marshal(Object{"command": command, "childProcesses": Obj(rendered.Response.Meta["counts"])["childProcesses"], "limits": limits})
		if e == nil {
			_, _ = io.WriteString(stderr, string(b)+"\n")
		}
	}
	if _, e = io.WriteString(stdout, rendered.Document); e != nil {
		return 1
	}
	if ctx.Err() != nil {
		return 130
	}
	return exit
}
