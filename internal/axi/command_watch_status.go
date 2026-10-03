package axi

import (
	"context"
	"time"
)

func WatchRun(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	id, interval := parsed.Positional, parsed.Int("interval", 10000)
	session, err := openCommandSession(ctx, ec, "watch")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	budget := Budget{Deadline: ec.Deadline, MaxChildProcesses: -1}
	policy := commandPolicy(session, ec, budget)
	started := time.Now().UnixMilli()
	var latest Object
	projectID := ""
	observed := ObservedAt()
	polls := 0
	outcome := "deadline"
	limits := []Limitation{}
	latestLimits := []Limitation{}
	for {
		if ctx.Err() != nil {
			return Response{}, NewError("INTERRUPTED", "Watcher interrupted", 1)
		}
		if time.Now().UnixMilli() >= ec.Deadline {
			limits = append(limits, Limitation{Code: "DEADLINE_EXCEEDED", Message: "Watch deadline reached; the latest available observation is retained", Source: "run", RunID: id})
			break
		}
		read := session.Reader.Read(ctx, ReadRequest{Kind: "run.detail", ID: id}, budget)
		polls++
		if read.State == "unavailable" {
			if read.Error.Code == "INTERRUPTED" {
				return Response{}, read.Error
			}
			outcome = "unavailable"
			switch read.Error.Code {
			case "NOT_FOUND":
				outcome = "vanished"
			case "AUTH_REQUIRED", "PERMISSION_DENIED":
				outcome = "inaccessible"
			case "DEADLINE_EXCEEDED":
				outcome = "deadline"
			}
			limits = append(limits, Limitation{Code: read.Error.Code, Message: "The watched execution could not be observed; retained state is the last verified observation", Source: "run", RunID: id})
			break
		}
		run := read.Value
		if (ec.Job != "" && Str(run, "jobId") != ec.Job) || (ec.Project != "" && read.Provenance.ProjectID != ec.Project) {
			return Response{}, NewError("CONTEXT_MISMATCH", "Watched execution belongs to another selected scope", 1)
		}
		if latest != nil && (latest["jobId"] != run["jobId"] || projectID != read.Provenance.ProjectID) {
			return Response{}, NewError("CONTEXT_MISMATCH", "Watched execution identity changed during polling", 1)
		}
		if err = policy.Assert(ctx, read.Provenance.ProjectID); err != nil {
			return Response{}, err
		}
		latest = run
		projectID = read.Provenance.ProjectID
		observed = read.Provenance.ObservedAt
		latestLimits = read.Provenance.Limitations
		if Str(run, "state") == "finished" {
			outcome = "finished"
			break
		}
		delay := min(int64(interval), ec.Deadline-time.Now().UnixMilli())
		if delay > 0 {
			timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Response{}, NewError("INTERRUPTED", "Watcher interrupted", 1)
			case <-timer.C:
			}
		}
	}
	limits = commandVersion(limits, session)
	limits = append(limits, latestLimits...)
	passed := outcome == "finished" && Str(latest, "result") == "success"
	var run any
	if latest != nil {
		run = projectObject(latest, []string{"id", "jobId", "state", "result", "branch", "personal", "composite", "revisions", "rawStatus", "startedAt", "finishedAt", "queuedAt"})
	}
	output := NewResponse("run.watch", Object{"runId": id, "run": run, "outcome": outcome, "check": Object{"requested": parsed.Bool("check"), "passed": passed}, "polling": Object{"count": polls, "intervalMs": interval, "elapsedMs": time.Now().UnixMilli() - started, "consistency": "best_effort"}})
	output.Context = Object{"server": ec.Server}
	if latest != nil {
		output.Context["job"] = latest["jobId"]
		output.Context["branch"] = valueOrNull(latest, "branch")
		if projectID != "" {
			output.Context["project"] = projectID
		}
	} else {
		if ec.Job != "" {
			output.Context["job"] = ec.Job
		}
		if ec.Project != "" {
			output.Context["project"] = ec.Project
		}
	}
	output.Meta["observedAt"] = observed
	commandCounts(&output, session)
	output.Meta["limits"] = PublicLimits(ReadLimits(ec, "watch"), commandMaxBytes(parsed, ec, 8192))
	commandLimitations(&output, limits)
	if !parsed.Bool("no-hints") && latest != nil {
		command := "view"
		if Str(latest, "state") == "finished" && containsString([]string{"failure", "error", "canceled", "failed_to_start"}, Str(latest, "result")) {
			command = "failure"
		}
		argv := []string{"teamcity-axi", "run", command, id, "--server=" + ec.Server, "--job=" + Str(latest, "jobId")}
		if projectID != "" {
			argv = append(argv, "--project="+projectID)
		}
		output.Next = []Object{commandHint("Inspect this exact observed execution", argv)}
	}
	return output, nil
}

func ReadStatus(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	if len(ec.Jobs) == 0 {
		return Response{}, NewError("CONTEXT_REQUIRED", "Select a job or repository tracked jobs for status", 2)
	}
	selected := append([]string{}, ec.Jobs[:min(5, len(ec.Jobs))]...)
	query := ReadRequest{Kind: "status.snapshot", JobIDs: selected, Branch: ec.Branch, BranchSet: ec.BranchDefined || ec.Branch != ""}
	if _, err := StatusRequest(query); err != nil {
		return Response{}, err
	}
	session, err := openCommandSession(ctx, ec, "status")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	read, err := commandRead(ctx, session, ec, query)
	if err != nil {
		return Response{}, err
	}
	policy := commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1})
	snapshots := map[string]Object{}
	limits := []Limitation{}
	for _, snapshot := range Objects(read.Value["snapshots"]) {
		job := Obj(snapshot["job"])
		if ec.Project != "" && Str(job, "projectId") != ec.Project {
			return Response{}, NewError("CONTEXT_MISMATCH", "A tracked status job belongs to another project", 1)
		}
		if err = policy.Assert(ctx, Str(job, "projectId")); err != nil {
			domain := AsDomainError(err)
			if domain.Code == "INTERRUPTED" {
				return Response{}, domain
			}
			limits = append(limits, Limitation{Code: domain.Code, Message: "A required job could not be admitted under the trusted project policy", Source: "context"})
		} else {
			snapshots[Str(job, "id")] = snapshot
		}
	}
	jobs := []Object{}
	for _, id := range selected {
		jobs = append(jobs, AssessStatusJob(id, snapshots[id], ec.Revision, ec.VCSRootID))
	}
	if len(ec.Jobs) > len(selected) {
		limits = append(limits, Limitation{Code: "TRACKED_JOBS_TRUNCATED", Message: "The bounded home view omits additional required jobs", Source: "context"})
	}
	if ec.Dirty == nil || *ec.Dirty {
		code, message := "WORKTREE_UNVERIFIED", "A clean local worktree could not be verified"
		if ec.Dirty != nil && *ec.Dirty {
			code, message = "DIRTY_WORKTREE", "Remote builds do not cover uncommitted local changes"
		}
		limits = append(limits, Limitation{Code: code, Message: message, Source: "context"})
	}
	if ec.Head == "" || ec.Revision != ec.Head {
		limits = append(limits, Limitation{Code: "CHECKOUT_IDENTITY_UNVERIFIED", Message: "The selected revision is not the verified local Git HEAD", Source: "context"})
	}
	if ec.VCSRootID == "" {
		limits = append(limits, Limitation{Code: "VCS_ROOT_UNVERIFIED", Message: "Select an exact VCS root or configure an unambiguous repository mapping", Source: "context"})
	}
	limits = commandVersion(limits, session)
	scopeComplete := len(snapshots) == len(selected) && len(ec.Jobs) == len(selected)
	checkoutVerified := ec.Dirty != nil && !*ec.Dirty && ec.Head != "" && ec.Revision == ec.Head
	passed := scopeComplete && checkoutVerified
	inProgress := scopeComplete && checkoutVerified
	anyFailed := false
	failedIDs := []string{}
	displayed := []Object{}
	for _, job := range jobs {
		assessment := Str(job, "assessment")
		if assessment != "passed" {
			passed = false
		}
		if assessment != "passed" && assessment != "in_progress" {
			inProgress = false
		}
		if assessment == "failed" {
			anyFailed = true
			failedIDs = append(failedIDs, Str(Obj(job["run"]), "id"))
		}
		notes := commandNotes(job["limitations"])
		limits = append(limits, notes...)
		shown := projectObject(job, keysOf(job))
		delete(shown, "limitations")
		var run any
		if raw := Obj(job["run"]); raw != nil {
			run = commandPrimaryRun(raw)
		}
		shown["run"] = run
		codes := []string{}
		for _, note := range notes {
			if !containsString(codes, note.Code) {
				codes = append(codes, note.Code)
			}
		}
		shown["limitationCodes"] = codes
		displayed = append(displayed, shown)
	}
	assessment := "unverified"
	if passed {
		assessment = "passed"
	} else if anyFailed {
		assessment = "failed"
	} else if inProgress {
		assessment = "in_progress"
	}
	output := NewResponse("status", Object{"mode": "configured", "checkout": Object{"head": nullString(ec.Head), "branch": nullString(ec.Branch), "dirty": ec.Dirty, "revision": nullString(ec.Revision), "vcsRootId": nullString(ec.VCSRootID)}, "assessment": assessment, "check": Object{"requested": parsed.Bool("check"), "passed": passed}, "jobs": displayed, "coverage": Object{"requiredJobs": len(ec.Jobs), "displayedJobs": len(jobs), "observedJobs": len(snapshots), "complete": scopeComplete, "candidateLimitPerJob": 20, "scanLimit": 5000, "ordering": "newest_run_id", "historyComplete": false, "consistency": "best_effort"}, "failedRunIds": failedIDs})
	output.Context = PublicContext(ec)
	delete(output.Context, "jobs")
	delete(output.Context, "job")
	output.Context["jobs"] = selected
	if ec.Job != "" && containsString(selected, ec.Job) {
		output.Context["job"] = ec.Job
	}
	dedup := []Limitation{}
	positions := map[string]int{}
	for _, l := range limits {
		if i, ok := positions[l.Code]; ok {
			dedup[i] = l
		} else {
			positions[l.Code] = len(dedup)
			dedup = append(dedup, l)
		}
	}
	commandLimitations(&output, dedup)
	commandCounts(&output, session)
	output.Meta["observedAt"] = read.Provenance.ObservedAt
	output.Meta["limits"] = PublicLimits(ReadLimits(ec, "status"), commandMaxBytes(parsed, ec, 6144))
	if !parsed.Bool("no-hints") {
		for _, job := range jobs {
			run := Obj(job["run"])
			if run == nil {
				continue
			}
			reason, command := "Inspect this exact status execution", "view"
			if Str(job, "assessment") == "failed" {
				reason, command = "Inspect this exact failed execution", "failure"
			}
			project := Str(Obj(snapshots[Str(job, "jobId")]["job"]), "projectId")
			output.Next = append(output.Next, commandHint(reason, []string{"teamcity-axi", "run", command, Str(run, "id"), "--server=" + ec.Server, "--job=" + Str(job, "jobId"), "--project=" + project}))
			if len(output.Next) == 3 {
				break
			}
		}
	}
	return output, nil
}
