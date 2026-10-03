package axi

import "context"

func ReadTree(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	runID, depth, maxNodes := parsed.Positional, parsed.Int("depth", 4), parsed.Int("max-nodes", 30)
	session, err := openCommandSession(ctx, ec, "graph")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	primaryBudget := Budget{Deadline: ec.Deadline, MaxChildProcesses: -1}
	read, err := commandRead(ctx, session, ec, ReadRequest{Kind: "run.detail", ID: runID})
	if err != nil {
		return Response{}, err
	}
	if err = commandAssertRunScope(parsed, read); err != nil {
		return Response{}, err
	}
	reserve := 0
	if Str(read.Value, "state") != "finished" {
		reserve = 1
	}
	budget := Budget{Deadline: ec.Deadline, MaxChildProcesses: max(0, session.MaxChildProcesses-reserve)}
	policy := commandPolicy(session, ec, budget)
	if err = policy.Assert(ctx, read.Provenance.ProjectID); err != nil {
		return Response{}, err
	}
	children := func() int {
		if session.Transport == nil {
			return 0
		}
		return session.Transport.ChildProcesses()
	}
	maxReads := max(0, session.MaxChildProcesses-children()-reserve)
	traversal, err := BuildGraph(ctx, session.Reader, ec, runID, GraphOptions{Root: read.Value, RootProjectID: read.Provenance.ProjectID, RootProjectKnown: true, Budget: budget, Depth: depth, MaxNodes: maxNodes, MaxGraphReads: maxReads, CanRead: func() bool { return children() < budget.MaxChildProcesses }, AssertProject: policy.AssertBudget})
	if err != nil {
		return Response{}, err
	}
	output := NewResponse("run.tree", Object{})
	output.Context = commandRunContext(ec, read)
	limits := append(append([]Limitation{}, read.Provenance.Limitations...), traversal.Limitations...)
	primary := read.Value
	if reserve > 0 {
		final := session.Reader.Read(ctx, ReadRequest{Kind: "run.detail", ID: runID}, primaryBudget)
		if final.State == "unavailable" {
			if final.Error.Code == "INTERRUPTED" {
				return Response{}, final.Error
			}
			markGraphRoot(traversal.Graph, runID, nil)
			limits = append(limits, Limitation{Code: final.Error.Code, Message: "Reserved final-state observation is unavailable", Source: "run", RunID: runID})
		} else {
			if final.Value["jobId"] != primary["jobId"] || final.Provenance.ProjectID != read.Provenance.ProjectID {
				return Response{}, NewError("CONTEXT_MISMATCH", "Final observation changed the frozen execution scope", 1)
			}
			primary = final.Value
			output.Context["branch"] = valueOrNull(primary, "branch")
			limits = append(limits, final.Provenance.Limitations...)
			if !SameGraphObservation(primary, read.Value) {
				markGraphRoot(traversal.Graph, runID, GraphRun(primary))
				limits = append(limits, Limitation{Code: "ROOT_STATE_CHANGED", Message: "Root lifecycle, result or scoped metadata changed while graph evidence was acquired", Source: "run", RunID: runID})
			}
			if Str(primary, "state") != "finished" {
				limits = append(limits, Limitation{Code: "PROVISIONAL_GRAPH", Message: "The execution remains non-terminal; observed topology can change", Source: "dependencies", RunID: runID})
			}
		}
	}
	output.Data = Object{"run": GraphRun(primary), "graph": traversal.Graph, "selection": Object{"depth": depth, "maxNodes": maxNodes, "maxGraphReads": maxReads, "graphReadAttempts": traversal.GraphReadAttempts, "omittedTargets": traversal.OmittedTargets, "consistency": "best_effort_counts_and_pages"}}
	limits = commandVersion(limits, session)
	commandLimitations(&output, limits, "UNVERIFIED_VERSION", "UNKNOWN_LIFECYCLE", "UNKNOWN_RESULT", "GRAPH_CYCLE_DETECTED")
	if !Bool(traversal.Graph, "complete") {
		output.Status = "partial"
		output.Meta["complete"] = false
	}
	commandCounts(&output, session)
	output.Meta["limits"] = PublicLimits(ReadLimits(ec, "graph"), commandMaxBytes(parsed, ec, 24576))
	if !parsed.Bool("no-hints") {
		for _, node := range Objects(traversal.Graph["nodes"]) {
			run := Obj(node["run"])
			id := Str(run, "id")
			if id != runID && Str(node, "expansion") != "complete" {
				argv := []string{"teamcity-axi", "run", "tree", id, "--server", ec.Server, "--job", Str(run, "jobId")}
				if project := traversal.Projects[id]; project != "" {
					argv = append(argv, "--project", project)
				}
				argv = append(argv, "--depth", numberArg(depth), "--max-nodes", numberArg(maxNodes))
				output.Next = []Object{commandHint("Inspect a retained unexpanded execution", argv)}
				break
			}
		}
	}
	return output, nil
}

func commandAssertRunScope(parsed Parsed, read ReadResult) error {
	if flagPresent(parsed, "job") && parsed.String("job") != Str(read.Value, "jobId") {
		return NewError("CONTEXT_MISMATCH", "Requested run belongs to another job", 1)
	}
	if flagPresent(parsed, "project") && parsed.String("project") != read.Provenance.ProjectID {
		return NewError("CONTEXT_MISMATCH", "Requested run belongs to another project", 1)
	}
	return nil
}

func markGraphRoot(graph Object, id string, run Object) {
	unexpanded := 0
	for _, node := range Objects(graph["nodes"]) {
		if Str(Obj(node["run"]), "id") == id {
			if run != nil {
				node["run"] = run
			}
			node["expansion"] = "unavailable"
		}
		if Str(node, "expansion") != "complete" {
			unexpanded++
		}
	}
	graph["complete"] = false
	graph["unexpanded"] = unexpanded
}

func ReadFailure(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	runID := parsed.Positional
	session, err := openCommandSession(ctx, ec, "graph")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	budget := Budget{Deadline: ec.Deadline, MaxChildProcesses: -1}
	primary, err := commandRead(ctx, session, ec, ReadRequest{Kind: "run.detail", ID: runID})
	if err != nil {
		return Response{}, err
	}
	if err = commandAssertRunScope(parsed, primary); err != nil {
		return Response{}, err
	}
	reserve := 0
	if Str(primary.Value, "state") != "finished" {
		reserve = 1
	}
	policy := commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: max(0, session.MaxChildProcesses-reserve)})
	if err = policy.Assert(ctx, primary.Provenance.ProjectID); err != nil {
		return Response{}, err
	}
	children := func() int {
		if session.Transport == nil {
			return 0
		}
		return session.Transport.ChildProcesses()
	}
	result, err := InvestigateFailure(ctx, session.Reader, ec, runID, FailureOptions{Primary: &primary, Budget: budget, MaxChildProcesses: session.MaxChildProcesses, ChildProcesses: children, Depth: parsed.Int("depth", 4), MaxNodes: parsed.Int("max-nodes", 30), MaxDiagnosedRuns: parsed.Int("max-diagnosed-runs", 3), Full: parsed.Bool("full"), AssertProject: policy.AssertBudget, Secrets: commandSecrets(ec)})
	if err != nil {
		return Response{}, err
	}
	data := Obj(result["data"])
	output := NewResponse("run.failure", data)
	output.Context = commandRunContext(ec, primary)
	output.Context["branch"] = valueOrNull(Obj(data["run"]), "branch")
	output.Meta["complete"] = Bool(result, "complete")
	output.Meta["truncated"] = Bool(result, "truncated")
	output.Meta["limitations"] = commandVersion(commandNotes(result["limitations"]), session)
	commandCounts(&output, session)
	output.Meta["limits"] = PublicLimits(ReadLimits(ec, "graph"), commandMaxBytes(parsed, ec, 24576))
	if !Bool(result, "complete") {
		output.Status = "partial"
	}
	if !parsed.Bool("no-hints") {
		for _, finding := range Objects(data["findings"]) {
			for _, evidence := range Objects(finding["evidence"]) {
				if Bool(Obj(result["truncatedEvidence"]), Str(evidence, "id")) {
					output.Next = append(output.Next, Obj(evidence["retrieve"]))
					break
				}
			}
			if len(output.Next) > 0 {
				break
			}
		}
		projects := Obj(result["projects"])
		typedProjects, _ := result["projects"].(map[string]string)
		for _, node := range Objects(Obj(data["graph"])["nodes"]) {
			run := Obj(node["run"])
			id := Str(run, "id")
			if id == runID || Str(node, "expansion") == "complete" {
				continue
			}
			argv := []string{"teamcity-axi", "run", "failure", id, "--server", ec.Server, "--job", Str(run, "jobId")}
			project := Str(projects, id)
			if project == "" {
				project = typedProjects[id]
			}
			if project != "" {
				argv = append(argv, "--project", project)
			}
			argv = append(argv, "--depth", numberArg(parsed.Int("depth", 4)), "--max-nodes", numberArg(parsed.Int("max-nodes", 30)), "--max-diagnosed-runs", numberArg(parsed.Int("max-diagnosed-runs", 3)))
			if parsed.Bool("full") {
				argv = append(argv, "--full")
			}
			output.Next = append(output.Next, commandHint("Inspect a retained incomplete dependency execution", argv))
			break
		}
	}
	return output, nil
}
