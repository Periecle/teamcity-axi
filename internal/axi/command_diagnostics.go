package axi

import (
	"context"
	"path/filepath"
	"time"
)

func LocalContext(ec ExecutionContext) Response {
	policy := "not_configured"
	if ec.Server != "" && commandAllowed(ec) != nil {
		policy = "not_checked"
	}
	output := NewResponse("context.show", Object{"scope": PublicContext(ec), "checkout": Object{"head": nullString(ec.Head), "branch": nullString(ec.Branch), "dirty": ec.Dirty}, "sources": ec.Sources, "readOnly": true, "verification": Object{"requested": false, "authentication": "not_checked", "identityFingerprint": nil, "jobs": []Object{}, "project": nil, "policy": policy, "nativeVersion": nil}})
	output.Context = PublicContext(ec)
	return output
}

func verifiedCommandScope(ctx context.Context, ec ExecutionContext, session *ReadSession) (Object, error) {
	ids := []string{}
	if ec.Job != "" {
		ids = append(ids, ec.Job)
	}
	for _, id := range ec.Jobs {
		if !containsString(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) > 5 {
		return nil, NewError("INPUT_LIMIT_EXCEEDED", "Context verification allows at most five selected jobs", 1)
	}
	jobs := []Object{}
	policy := commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1})
	for _, id := range ids {
		read, err := commandRead(ctx, session, ec, ReadRequest{Kind: "job.detail", ID: id})
		if err != nil {
			return nil, err
		}
		if ec.Project != "" && Str(read.Value, "projectId") != ec.Project {
			return nil, NewError("CONTEXT_MISMATCH", "Selected job belongs to a different project", 1)
		}
		if err = policy.Assert(ctx, Str(read.Value, "projectId")); err != nil {
			return nil, err
		}
		job := projectObject(read.Value, keysOf(read.Value))
		name, _ := boundedText(Str(job, "name"), 200)
		job["name"] = name
		jobs = append(jobs, job)
	}
	projectID := ec.Project
	if projectID == "" && len(jobs) == 1 {
		projectID = Str(jobs[0], "projectId")
	}
	var project any
	if projectID != "" {
		value, err := policy.Project(ctx, projectID)
		if err != nil {
			return nil, err
		}
		project = value
		if err = policy.Assert(ctx, projectID); err != nil {
			return nil, err
		}
	}
	identity, err := commandRead(ctx, session, ec, ReadRequest{Kind: "identity.current"})
	if err != nil {
		return nil, err
	}
	return Object{"jobs": jobs, "project": project, "identityFingerprint": identity.Value["fingerprint"]}, nil
}

func Diagnose(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	offline := parsed.Descriptor.Name == "doctor" && parsed.Bool("offline")
	if !offline && ec.Server == "" {
		return Response{}, NewError("CONTEXT_REQUIRED", "Select a registered trusted server", 2)
	}
	if !offline && parsed.Descriptor.Name == "doctor" && ec.Project == "" && ec.Job == "" && len(ec.Jobs) == 0 {
		return Response{}, NewError("CONTEXT_REQUIRED", "Online doctor requires a selected project or job for bounded probes", 2)
	}
	profile := "simple"
	if offline {
		profile = "offline"
	}
	session, err := openCommandSession(ctx, ec, profile)
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	var scope Object
	if !offline {
		scope, err = verifiedCommandScope(ctx, ec, session)
		if err != nil {
			return Response{}, err
		}
	}
	var output Response
	limits := []Limitation{}
	if parsed.Descriptor.Name == "context.show" {
		output = LocalContext(ec)
		policy := "not_configured"
		if commandAllowed(ec) != nil {
			policy = "not_checked"
			if len(Objects(scope["jobs"])) > 0 || scope["project"] != nil {
				policy = "verified"
			}
		}
		output.Data["verification"] = Object{"requested": true, "authentication": "authenticated", "identityFingerprint": scope["identityFingerprint"], "jobs": scope["jobs"], "project": scope["project"], "policy": policy, "nativeVersion": session.NativeVersion}
		if policy == "not_checked" {
			limits = append(limits, Limitation{Code: "POLICY_SCOPE_UNVERIFIED", Message: "Authentication is verified; select a project or job to verify narrowing policy", Source: "context"})
		}
	} else {
		names := []string{"structuredRunDetail", "boundedRunPages", "independentProblemPages", "independentTestPages", "snapshotDependencyPages", "structuredLogTail", "boundedChangesPages", "scopedQueueRead", "safeAgentRead"}
		capabilities := []Object{}
		for _, name := range names {
			capabilities = append(capabilities, Object{"name": name, "required": name != "structuredLogTail", "state": "not_probed"})
		}
		set := func(name string, values Object) {
			for _, c := range capabilities {
				if Str(c, "name") == name {
					for k, v := range values {
						c[k] = v
					}
					return
				}
			}
		}
		var serverInfo any
		if !offline {
			read, err := commandRead(ctx, session, ec, ReadRequest{Kind: "server.detail"})
			if err != nil {
				return Response{}, err
			}
			serverInfo = read.Value
			jobs := Objects(scope["jobs"])
			jobID, projectID := "", ""
			if len(jobs) > 0 {
				jobID = Str(jobs[0], "id")
				projectID = Str(jobs[0], "projectId")
			} else {
				projectID = Str(Obj(scope["project"]), "id")
			}
			query := ReadRequest{Kind: "run.page", JobID: jobID, ProjectID: projectID, Count: 1, ScanLimit: 5000}
			if commandAllowed(ec) != nil {
				query.AllowedProjects = []string{projectID}
			}
			page, err := commandRead(ctx, session, ec, query)
			if err != nil {
				return Response{}, err
			}
			set("boundedRunPages", Object{"state": "available"})
			runs := Objects(page.Value["runs"])
			if runs == nil {
				runs = Objects(page.Value["items"])
			}
			if len(runs) > 0 {
				sample := runs[0]
				detail, err := commandRead(ctx, session, ec, ReadRequest{Kind: "run.detail", ID: Str(sample, "id")})
				if err != nil {
					return Response{}, err
				}
				if detail.Value["jobId"] != sample["jobId"] || detail.Provenance.ProjectID != projectID {
					return Response{}, NewError("CONTEXT_MISMATCH", "Capability probe changed execution scope", 1)
				}
				if err = commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1}).Assert(ctx, detail.Provenance.ProjectID); err != nil {
					return Response{}, err
				}
				set("structuredRunDetail", Object{"state": "available", "runId": sample["id"]})
				log, err := commandRead(ctx, session, ec, ReadRequest{Kind: "log.tail", ID: Str(sample, "id"), Tail: 1})
				if err != nil {
					domain := AsDomainError(err)
					if domain.Code == "INTERRUPTED" {
						return Response{}, err
					}
					set("structuredLogTail", Object{"state": "unavailable", "errorCode": domain.Code, "runId": sample["id"]})
				} else {
					_ = log
					set("structuredLogTail", Object{"state": "available", "runId": sample["id"]})
				}
			}
		}
		versionStatus := "unverified_version"
		if session.NativeVersion == "1.5.0" {
			versionStatus = "recorded"
		}
		authentication, policy := "not_checked", "not_checked"
		var fingerprint any
		if !offline {
			authentication = "authenticated"
			fingerprint = scope["identityFingerprint"]
			policy = "not_configured"
			if commandAllowed(ec) != nil {
				policy = "verified"
			}
		}
		publicLimits := PublicLimits(ReadLimits(ec, "simple"), commandMaxBytes(parsed, ec, 16384))
		delete(publicLimits, "deadline")
		publicLimits["remainingMs"] = max(int64(0), ec.Deadline-time.Now().UnixMilli())
		output = NewResponse("doctor", Object{"offline": offline, "executable": Object{"name": filepath.Base(session.Binary), "resolved": true, "version": session.NativeVersion, "versionStatus": versionStatus}, "target": Object{"registered": ec.Server != ""}, "server": serverInfo, "authentication": Object{"state": authentication, "identityFingerprint": fingerprint}, "policy": Object{"readOnly": true, "state": policy}, "liveCertified": false, "output": Object{"formats": []string{"json", "toon"}, "schemaVersion": "1.0"}, "capabilities": capabilities, "limits": publicLimits})
		output.Context = PublicContext(ec)
		unprobed := false
		for _, c := range capabilities {
			if Bool(c, "required") && Str(c, "state") == "not_probed" {
				unprobed = true
			}
		}
		if !offline && unprobed {
			limits = append(limits, Limitation{Code: "CAPABILITIES_NOT_PROBED", Message: "Capabilities without a scoped adapter or retained sample remain unverified", Source: "context"})
		}
		for _, c := range capabilities {
			if Str(c, "name") == "structuredLogTail" && Str(c, "state") == "unavailable" {
				limits = append(limits, Limitation{Code: "OPTIONAL_LOG_UNAVAILABLE", Message: "Structured log tail is unavailable; metadata reads remain usable", Source: "log"})
				if containsString([]string{"DEADLINE_EXCEEDED", "INPUT_LIMIT_EXCEEDED"}, Str(c, "errorCode")) {
					limits = append(limits, Limitation{Code: Str(c, "errorCode"), Message: "Optional log probe exhausted its shared budget; acquired metadata is retained", Source: "log"})
				}
			}
		}
	}
	limits = commandVersion(limits, session)
	commandCounts(&output, session)
	commandLimitations(&output, limits, "UNVERIFIED_VERSION", "OPTIONAL_LOG_UNAVAILABLE")
	return output, nil
}
