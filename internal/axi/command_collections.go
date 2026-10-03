package axi

import (
	"context"
	"time"
)

func ViewJob(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	id := parsed.Positional
	if id == "" || len(id) > 256 {
		return Response{}, Usage("Job ID exceeds its identity bound")
	}
	if _, err := Literal(id); err != nil {
		return Response{}, err
	}
	session, err := openCommandSession(ctx, ec, "simple")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	read, err := commandRead(ctx, session, ec, ReadRequest{Kind: "job.detail", ID: id})
	if err != nil {
		return Response{}, err
	}
	project := Str(read.Value, "projectId")
	if flagPresent(parsed, "project") && parsed.String("project") != project {
		return Response{}, NewError("CONTEXT_MISMATCH", "Requested job belongs to another project", 1)
	}
	if err = commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1}).Assert(ctx, project); err != nil {
		return Response{}, err
	}
	output := NewResponse("job.view", Object{"job": read.Value})
	output.Context = Object{"server": ec.Server, "job": read.Value["id"], "project": project}
	output.Meta["observedAt"] = read.Provenance.ObservedAt
	output.Next = []Object{commandHint("Read executions for this exact job", []string{"teamcity-axi", "run", "list", "--server", ec.Server, "--project=" + project, "--job=" + Str(read.Value, "id"), "--all-branches"})}
	return commandFinish(output, parsed, ec, session, 16384, read.Provenance.Limitations, "BEST_EFFORT_PAGINATION", "UNVERIFIED_VERSION"), nil
}

func ListJobs(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	if ec.Project == "" {
		return Response{}, NewError("CONTEXT_REQUIRED", "Select an exact project for job list", 2)
	}
	return listScopedCollection(ctx, parsed, ec, "job")
}

func ListQueue(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	if ec.Project == "" && ec.Job == "" {
		return Response{}, NewError("CONTEXT_REQUIRED", "Select a job or project for queue list", 2)
	}
	return listScopedCollection(ctx, parsed, ec, "queue")
}

func listScopedCollection(ctx context.Context, parsed Parsed, ec ExecutionContext, kind string) (Response, error) {
	now := time.Now().UnixMilli()
	cursor, err := commandCursor(parsed, now)
	if err != nil {
		return Response{}, err
	}
	query := ReadRequest{Kind: kind + ".page", ProjectID: ec.Project, Count: parsed.Int("limit", 20), ScanLimit: 5000}
	if kind == "queue" {
		query.JobID = ec.Job
	}
	if cursor != nil {
		query.Start = cursor.Position
	}
	request := JobRequest
	if kind == "queue" {
		request = QueueRequest
	}
	if _, err = request(query); err != nil {
		return Response{}, err
	}
	bind := func() (CursorBinding, error) {
		r, e := request(query)
		return CursorBinding{Command: kind + ".list", Server: ec.Server, Count: query.Count, FilterHash: commandHash(Object{"serverUrl": ec.ServerURL, "filters": r.Filters, "allowedProjects": sortedAllowed(ec)})}, e
	}
	if cursor != nil {
		if kind == "job" || query.ProjectID != "" {
			bound, e := bind()
			if e != nil {
				return Response{}, e
			}
			if e = AssertCursor(*cursor, bound); e != nil {
				return Response{}, e
			}
		}
		if cursor.Command != kind+".list" || cursor.Server != ec.Server || cursor.Count != query.Count {
			return Response{}, Usage("Cursor belongs to another " + kind + " query")
		}
	}
	session, err := openCommandSession(ctx, ec, "simple")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	query, err = commandOwnedScope(ctx, session, ec, query, "Selected queue job belongs to another project", true)
	if err != nil {
		return Response{}, err
	}
	bound, err := bind()
	if err != nil {
		return Response{}, err
	}
	if cursor != nil {
		if err = AssertCursor(*cursor, bound); err != nil {
			return Response{}, err
		}
	}
	read, err := commandRead(ctx, session, ec, query)
	if err != nil {
		return Response{}, err
	}
	page := read.Value
	items := Objects(page["items"])
	if items == nil {
		items = []Object{}
	}
	message := "Offset pages can change when jobs are created, moved or removed"
	if kind == "queue" {
		message = "Queued executions can start, leave or change position between observations"
	}
	limits := append(pageLimitations(page), Limitation{Code: "BEST_EFFORT_PAGINATION", Message: message, Source: kind})
	token, note, err := continuation(bound, cursor, page, now)
	if err != nil {
		return Response{}, err
	}
	if note != nil {
		note.Source = kind
		limits = append(limits, *note)
	}
	selection := Object{"projectId": query.ProjectID, "pageSize": query.Count, "providerReturned": page["providerReturned"], "position": query.Start, "scanLimit": 5000, "consistency": "best_effort_offset"}
	key := "jobs"
	if kind == "job" {
		selection["membership"] = "direct"
	} else {
		key = "items"
		selection["jobId"] = nullString(query.JobID)
		selection["meaning"] = "queued_executions"
	}
	data := Object{key: items, "page": commandPage(items, page, token), "selection": selection}
	if len(items) == 0 {
		reason := "No jobs returned; bounded collection exhaustion is unverified"
		if page["hasMore"] == true {
			reason = "No jobs returned in this page; scoped continuation remains"
		}
		if kind == "queue" {
			reason = "No queued items returned; bounded queue exhaustion is unverified"
			if page["hasMore"] == true {
				reason = "No items returned in this page; scoped continuation remains"
			}
		}
		data["emptyReason"] = reason
	}
	output := NewResponse(kind+".list", data)
	output.Context = Object{"server": ec.Server, "project": query.ProjectID}
	if query.JobID != "" {
		output.Context["job"] = query.JobID
	}
	output.Meta["observedAt"] = read.Provenance.ObservedAt
	if token != nil {
		argv := []string{"teamcity-axi", kind, "list", "--server", ec.Server, "--project=" + query.ProjectID}
		if query.JobID != "" {
			argv = append(argv, "--job="+query.JobID)
		}
		argv = append(argv, "--limit", numberArg(query.Count), "--cursor", Text(token))
		reason := "Read the next bounded page for this exact project"
		if kind == "queue" {
			reason = "Read the next bounded page of this queue scope"
		}
		output.Next = []Object{commandHint(reason, argv)}
	} else {
		for i, item := range items {
			if i == 2 {
				break
			}
			argv := []string{"teamcity-axi", "job", "view", "--server", ec.Server, "--project=" + query.ProjectID, "--", Str(item, "id")}
			reason := "Inspect safe metadata for this job"
			if kind == "queue" {
				argv = []string{"teamcity-axi", "run", "view", Str(item, "id"), "--server", ec.Server, "--project=" + query.ProjectID, "--job=" + Str(item, "jobId")}
				reason = "Observe this exact queued execution"
			}
			output.Next = append(output.Next, commandHint(reason, argv))
		}
	}
	return commandFinish(output, parsed, ec, session, 16384, limits, "BEST_EFFORT_PAGINATION", "UNVERIFIED_VERSION"), nil
}

func ReadAgents(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	list := parsed.Descriptor.Name == "agent.list"
	now := time.Now().UnixMilli()
	cursor, err := commandCursor(parsed, now)
	if err != nil {
		return Response{}, err
	}
	query := ReadRequest{Kind: "agents.page", JobID: ec.Job, ProjectID: ec.Project, PoolID: parsed.String("pool"), Count: parsed.Int("limit", 20), ScanLimit: 5000}
	if cursor != nil {
		query.Start = cursor.Position
	}
	if list {
		if _, err = AgentRequest(query); err != nil {
			return Response{}, err
		}
	} else {
		query.Kind = "agent.detail"
		query.ID = parsed.Positional
		if err = ValidateAgentID(parsed.Positional, false); err != nil {
			return Response{}, err
		}
		if _, err = AgentFilters(query); err != nil {
			return Response{}, err
		}
	}
	scope := func() Object {
		o := Object{}
		if query.JobID != "" {
			o["jobId"] = query.JobID
		}
		if query.ProjectID != "" {
			o["projectId"] = query.ProjectID
		}
		if flagPresent(parsed, "pool") {
			o["poolId"] = query.PoolID
		}
		return o
	}
	bind := func() CursorBinding {
		return CursorBinding{Command: "agent.list", Server: ec.Server, Count: query.Count, FilterHash: commandHash(Object{"serverUrl": ec.ServerURL, "scope": scope(), "allowedProjects": sortedAllowed(ec)})}
	}
	if cursor != nil {
		if query.JobID == "" || query.ProjectID != "" {
			if err = AssertCursor(*cursor, bind()); err != nil {
				return Response{}, err
			}
		}
		if cursor.Command != "agent.list" || cursor.Server != ec.Server || cursor.Count != query.Count {
			return Response{}, Usage("Cursor belongs to another agent query")
		}
	}
	session, err := openCommandSession(ctx, ec, "simple")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	query, err = commandOwnedScope(ctx, session, ec, query, "Selected agent job belongs to another project", false)
	if err != nil {
		return Response{}, err
	}
	if cursor != nil {
		if err = AssertCursor(*cursor, bind()); err != nil {
			return Response{}, err
		}
	}
	read, err := commandRead(ctx, session, ec, query)
	if err != nil {
		return Response{}, err
	}
	limits := append([]Limitation{}, read.Provenance.Limitations...)
	note := func(code, message string) {
		for _, l := range limits {
			if l.Code == code {
				return
			}
		}
		limits = append(limits, Limitation{Code: code, Message: message, Source: "agent"})
	}
	agents := []Object{read.Value}
	page := Object(nil)
	if list {
		page = read.Value
		agents = Objects(page["items"])
		if agents == nil {
			agents = []Object{}
		}
	}
	policy := commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1})
	for _, agent := range agents {
		active := Obj(agent["activeRun"])
		if active == nil {
			continue
		}
		if err = policy.Assert(ctx, Str(active, "projectId")); err != nil {
			if AsDomainError(err).Code == "INTERRUPTED" {
				return Response{}, err
			}
			agent["activeRun"] = nil
			agent["activeRunState"] = "unavailable"
			note("ACTIVE_RUN_SCOPE_UNVERIFIED", "The active execution pointer could not be admitted by current project policy")
		}
	}
	if session.NativeVersion != "1.5.0" {
		note("UNVERIFIED_VERSION", "Native version has not been release-certified")
	}
	var token any
	if list {
		note("BEST_EFFORT_PAGINATION", "Agent state and compatibility can change between page observations")
		var n *Limitation
		token, n, err = continuation(bind(), cursor, page, now)
		if err != nil {
			return Response{}, err
		}
		if n != nil {
			note(n.Code, "Agent continuation expired during acquisition")
		}
	}
	meaning := "exact_agent"
	if list {
		meaning = "pool_agents"
	}
	if query.JobID != "" || query.ProjectID != "" {
		meaning = "compatible_agents"
	}
	selection := Object{"projectId": nullString(query.ProjectID), "jobId": nullString(query.JobID), "poolId": nullString(query.PoolID), "meaning": meaning}
	data := Object{"selection": selection}
	if list {
		extra := Object{"pageSize": query.Count, "providerReturned": page["providerReturned"], "position": query.Start, "scanLimit": 5000, "consistency": "best_effort_offset"}
		for k, v := range extra {
			selection[k] = v
		}
		data = Object{"agents": agents, "selection": selection, "page": commandPage(agents, page, token)}
		if len(agents) == 0 {
			data["emptyReason"] = "No agents returned; bounded scope exhaustion is unverified"
		}
	} else {
		data["agent"] = agents[0]
	}
	output := NewResponse(parsed.Descriptor.Name, data)
	output.Context = Object{"server": ec.Server}
	if query.ProjectID != "" {
		output.Context["project"] = query.ProjectID
	}
	if query.JobID != "" {
		output.Context["job"] = query.JobID
	}
	output.Meta["observedAt"] = read.Provenance.ObservedAt
	scopeFlags := []string{"--server", ec.Server}
	if query.ProjectID != "" {
		scopeFlags = append(scopeFlags, "--project="+query.ProjectID)
	}
	if query.JobID != "" {
		scopeFlags = append(scopeFlags, "--job="+query.JobID)
	}
	if flagPresent(parsed, "pool") {
		scopeFlags = append(scopeFlags, "--pool="+query.PoolID)
	}
	if token != nil {
		argv := append([]string{"teamcity-axi", "agent", "list"}, scopeFlags...)
		argv = append(argv, "--limit", numberArg(query.Count), "--cursor", Text(token))
		output.Next = []Object{commandHint("Continue this bounded agent scope", argv)}
	} else if list {
		for i, agent := range agents {
			if i == 2 {
				break
			}
			argv := append([]string{"teamcity-axi", "agent", "view", Str(agent, "id")}, scopeFlags...)
			output.Next = append(output.Next, commandHint("Observe this exact agent in the same scope", argv))
		}
	} else if active := Obj(agents[0]["activeRun"]); active != nil {
		output.Next = []Object{commandHint("Observe this reported active execution", []string{"teamcity-axi", "run", "view", Str(active, "id"), "--server", ec.Server, "--job=" + Str(active, "jobId"), "--project=" + Str(active, "projectId")})}
	}
	commandCounts(&output, session)
	output.Meta["limits"] = PublicLimits(ReadLimits(ec, "simple"), commandMaxBytes(parsed, ec, 16384))
	commandLimitations(&output, limits, "BEST_EFFORT_PAGINATION", "UNVERIFIED_VERSION")
	if parsed.Bool("no-hints") {
		output.Next = nil
	}
	return output, nil
}
