package axi

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

// GraphOptions bounds discovery independently from the shared process budget.
type GraphOptions struct {
	Root             Object
	RootProjectID    string
	RootProjectKnown bool
	Budget           Budget
	Depth            int
	MaxNodes         int
	MaxGraphReads    int
	CanRead          func() bool
	AssertProject    func(context.Context, string, Budget) error
}

type GraphReport struct {
	Graph             Object
	Limitations       []Limitation
	Depths            map[string]int
	Projects          map[string]string
	Observations      map[string]string
	GraphReadAttempts int
	OmittedTargets    int
}

type graphNode struct {
	run                  Object
	expansion            string
	dependencyCount      *int
	observedDependencies *int
	depth                int
	observedAt           string
	projectID            string
	projectKnown         bool
}

// GraphRun retains only the normalized execution fields permitted in graphs.
func GraphRun(run Object) Object {
	result := Object{"id": run["id"], "jobId": run["jobId"], "state": run["state"], "result": run["result"], "branch": nil}
	if branch, ok := run["branch"]; ok {
		result["branch"] = branch
	}
	for _, key := range []string{"revisions", "personal", "composite"} {
		if value, ok := run[key]; ok {
			result[key] = value
		}
	}
	if Str(run, "result") == "unknown" {
		result["rawStatus"] = run["rawStatus"]
	}
	return result
}

func SameGraphObservation(left, right Object) bool {
	comparable := func(run Object) []byte {
		value := GraphRun(run)
		if revisions, exists := run["revisions"]; exists {
			ordered := append([]Object{}, Objects(revisions)...)
			sort.SliceStable(ordered, func(i, j int) bool {
				if Str(ordered[i], "vcsRootId") != Str(ordered[j], "vcsRootId") {
					return Str(ordered[i], "vcsRootId") < Str(ordered[j], "vcsRootId")
				}
				return Str(ordered[i], "revision") < Str(ordered[j], "revision")
			})
			value["revisions"] = ordered
		}
		encoded, _ := json.Marshal(value)
		return encoded
	}
	return string(comparable(left)) == string(comparable(right))
}

func numericRunLess(left, right string) bool {
	a, _ := strconv.ParseUint(left, 10, 64)
	b, _ := strconv.ParseUint(right, 10, 64)
	return a < b
}

func graphCycles(nodes, edges []Object, rootID string) []Object {
	visited, path := map[string]bool{}, map[string]bool{}
	outgoing := map[string][]Object{}
	result := []Object{}
	for _, edge := range edges {
		id := Str(edge, "fromRunId")
		outgoing[id] = append(outgoing[id], edge)
	}
	var visit func(string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		visited[id], path[id] = true, true
		for _, edge := range outgoing[id] {
			target := Str(edge, "toRunId")
			if path[target] {
				result = append(result, edge)
			} else {
				visit(target)
			}
		}
		delete(path, id)
	}
	visit(rootID)
	for _, node := range nodes {
		visit(Str(Obj(node["run"]), "id"))
	}
	return result
}

func plannerBudget(ec ExecutionContext, budget Budget) Budget {
	if budget.Deadline == 0 {
		budget.Deadline = ec.Deadline
	}
	return budget
}

func readFailure(read ReadResult) *DomainError {
	if read.Error != nil {
		return read.Error
	}
	return NewError("UPSTREAM_ERROR", "Independent source is unavailable", 1)
}

func plannerLimitations(value any) []Limitation {
	if result, ok := value.([]Limitation); ok {
		return result
	}
	result := []Limitation{}
	for _, note := range Objects(value) {
		result = append(result, Limitation{Code: Str(note, "code"), Message: Str(note, "message"), RunID: Str(note, "runId"), Source: Str(note, "source")})
	}
	return result
}

// BuildGraph follows snapshot prerequisites while preserving every unverified boundary.
func BuildGraph(ctx context.Context, reader Reader, ec ExecutionContext, runID string, options GraphOptions) (GraphReport, error) {
	report := GraphReport{Limitations: []Limitation{}, Depths: map[string]int{}, Projects: map[string]string{}, Observations: map[string]string{}}
	if options.Depth < 0 || options.Depth > 12 || options.MaxNodes < 1 || options.MaxNodes > 200 || options.MaxGraphReads < 0 || options.MaxGraphReads > 24 {
		return report, Usage("Invalid bounded graph request")
	}
	options.Budget = plannerBudget(ec, options.Budget)
	if options.Root == nil {
		read := reader.Read(ctx, ReadRequest{Kind: "run.detail", ID: runID}, options.Budget)
		if read.State == "unavailable" {
			return report, readFailure(read)
		}
		options.Root = read.Value
		options.RootProjectID, options.RootProjectKnown = read.Provenance.ProjectID, true
	}
	root := &graphNode{run: options.Root, expansion: "complete", observedAt: ObservedAt(), projectID: options.RootProjectID, projectKnown: options.RootProjectKnown || options.RootProjectID != ""}
	rootID := Str(root.run, "id")
	nodes := map[string]*graphNode{rootID: root}
	queue := []*graphNode{root}
	edges, omitted, sharedLimits := map[string]Object{}, map[string]bool{}, map[string]bool{}
	report.Depths[rootID] = 0
	rank := map[string]int{"complete": 0, "not_requested": 0, "depth_limit": 1, "node_limit": 2, "call_limit": 3, "unavailable": 4, "permission_denied": 5}
	mark := func(node *graphNode, expansion string) {
		if rank[expansion] > rank[node.expansion] {
			node.expansion = expansion
		}
	}
	limit := func(node *graphNode, code, message, expansion string) {
		mark(node, expansion)
		shared := expansion == "depth_limit" || expansion == "node_limit" || expansion == "call_limit"
		if shared && sharedLimits[code] {
			return
		}
		if shared {
			sharedLimits[code] = true
		}
		note := Limitation{Code: code, Message: message, Source: "dependencies"}
		if !shared {
			note.RunID = Str(node.run, "id")
		}
		report.Limitations = append(report.Limitations, note)
	}
	failed := func(node *graphNode, err error) error {
		domain := AsDomainError(err)
		if domain.Code == "INTERRUPTED" {
			return domain
		}
		expansion := "unavailable"
		switch domain.Code {
		case "POLICY_DENIED", "PERMISSION_DENIED":
			expansion = "permission_denied"
		case "CALL_LIMIT_EXCEEDED", "DEADLINE_EXCEEDED":
			expansion = "call_limit"
		}
		limit(node, domain.Code, "An independent graph read or scoped target is unavailable", expansion)
		return nil
	}
	spend := func() error {
		if ctx.Err() != nil {
			return NewError("INTERRUPTED", "Graph observation interrupted", 130)
		}
		if time.Now().UnixMilli() >= options.Budget.Deadline {
			return NewError("DEADLINE_EXCEEDED", "Shared graph deadline exhausted", 1)
		}
		if report.GraphReadAttempts >= options.MaxGraphReads || (options.CanRead != nil && !options.CanRead()) {
			return NewError("CALL_LIMIT_EXCEEDED", "Shared graph read capacity exhausted", 1)
		}
		report.GraphReadAttempts++
		return nil
	}
	for index := 0; index < len(queue); index++ {
		node := queue[index]
		id := Str(node.run, "id")
		if node.depth >= options.Depth {
			limit(node, "GRAPH_DEPTH_LIMIT", "Dependencies were not requested beyond the declared depth", "depth_limit")
			continue
		}
		if err := spend(); err != nil {
			if fatal := failed(node, err); fatal != nil {
				return report, fatal
			}
			continue
		}
		count := reader.Read(ctx, ReadRequest{Kind: "dependencies.count", ID: id, RunID: id}, options.Budget)
		if count.State == "unavailable" {
			err := readFailure(count)
			if fatal := failed(node, err); fatal != nil {
				return report, fatal
			}
			if err.Code == "CALL_LIMIT_EXCEEDED" || err.Code == "DEADLINE_EXCEEDED" {
				continue
			}
		} else {
			value := Int(count.Value, "count")
			node.dependencyCount = &value
			if value == 0 {
				zero := 0
				node.observedDependencies = &zero
				continue
			}
		}
		observed, start := map[string]bool{}, 0
		for {
			if err := spend(); err != nil {
				if fatal := failed(node, err); fatal != nil {
					return report, fatal
				}
				break
			}
			read := reader.Read(ctx, ReadRequest{Kind: "dependencies.page", RunID: id, Count: 100, Start: start, ScanLimit: 5000}, options.Budget)
			if read.State == "unavailable" {
				if fatal := failed(node, readFailure(read)); fatal != nil {
					return report, fatal
				}
				break
			}
			page := read.Value
			accepted, duplicate := map[string]bool{}, false
			observedCount := len(observed)
			node.observedDependencies = &observedCount
			children := append([]Object{}, Objects(page["items"])...)
			sort.SliceStable(children, func(i, j int) bool {
				return numericRunLess(Str(Obj(children[i]["run"]), "id"), Str(Obj(children[j]["run"]), "id"))
			})
			for _, child := range children {
				run, project := Obj(child["run"]), Str(child, "projectId")
				childID := Str(run, "id")
				if observed[childID] {
					duplicate = true
					continue
				}
				observed[childID] = true
				value := len(observed)
				node.observedDependencies = &value
				if options.AssertProject != nil {
					if err := options.AssertProject(ctx, project, options.Budget); err != nil {
						if fatal := failed(node, err); fatal != nil {
							return report, fatal
						}
						continue
					}
				}
				target := nodes[childID]
				if target != nil && (Str(target.run, "jobId") != Str(run, "jobId") || (target.projectKnown && target.projectID != project)) {
					limit(node, "CONTEXT_MISMATCH", "Conflicting execution identities were returned for a shared graph node", "unavailable")
					continue
				}
				if target == nil {
					if len(nodes) >= options.MaxNodes {
						omitted[childID] = true
						limit(node, "GRAPH_NODE_LIMIT", "Discovered target could not be retained within the unique-node limit", "node_limit")
						continue
					}
					target = &graphNode{run: run, projectID: project, projectKnown: true, depth: node.depth + 1, observedAt: read.Provenance.ObservedAt, expansion: "complete"}
					nodes[childID] = target
					queue = append(queue, target)
					report.Depths[childID] = target.depth
				} else if !SameGraphObservation(target.run, run) {
					limit(node, "RUN_STATE_CHANGED", "A shared execution changed lifecycle, result or scoped metadata between observations", "unavailable")
				}
				accepted[childID] = true
				key := id + ":" + childID
				if _, exists := edges[key]; !exists {
					if len(edges) >= 2000 {
						limit(node, "GRAPH_EDGE_LIMIT", "Graph edge ceiling reached", "node_limit")
						continue
					}
					edges[key] = Object{"fromRunId": id, "toRunId": childID, "kind": "snapshot"}
				}
			}
			unsafe := false
			for _, note := range plannerLimitations(page["limitations"]) {
				if note.Code == "SCAN_COVERAGE_UNKNOWN" || (note.Source == "run" && !accepted[note.RunID]) {
					continue
				}
				note.Source = "dependencies"
				if note.RunID == "" {
					note.RunID = id
				}
				report.Limitations = append(report.Limitations, note)
				unsafe = unsafe || note.Code == "UNSAFE_CONTINUATION"
			}
			if duplicate {
				limit(node, "DUPLICATE_DEPENDENCY", "A dependency appeared repeatedly across offset pages", "unavailable")
				break
			}
			if unsafe {
				mark(node, "unavailable")
				break
			}
			if node.dependencyCount != nil && len(observed) >= *node.dependencyCount {
				if len(observed) != *node.dependencyCount || Bool(page, "hasMore") {
					limit(node, "DEPENDENCY_COUNT_MISMATCH", "Dependency pages conflict with the independently observed count", "unavailable")
				}
				break
			}
			if page["position"] == nil {
				if node.dependencyCount != nil {
					limit(node, "DEPENDENCY_COUNT_MISMATCH", "Returned dependencies do not reconcile with the scoped count", "unavailable")
				} else {
					limit(node, "DEPENDENCY_COUNT_UNAVAILABLE", "Observed rows do not establish complete expansion without a scoped count", "unavailable")
				}
				break
			}
			start = Int(page, "position")
		}
	}
	retained := append([]*graphNode{}, queue...)
	sort.SliceStable(retained, func(i, j int) bool {
		if retained[i].depth != retained[j].depth {
			return retained[i].depth < retained[j].depth
		}
		return numericRunLess(Str(retained[i].run, "id"), Str(retained[j].run, "id"))
	})
	publicNodes, publicEdges := []Object{}, []Object{}
	unexpanded := 0
	for _, node := range retained {
		var count, observed any
		if node.dependencyCount != nil {
			count = *node.dependencyCount
		}
		if node.observedDependencies != nil {
			observed = *node.observedDependencies
		}
		publicNodes = append(publicNodes, Object{"run": GraphRun(node.run), "expansion": node.expansion, "dependencyCount": count, "observedDependencies": observed})
		if node.expansion != "complete" {
			unexpanded++
		}
		id := Str(node.run, "id")
		report.Projects[id], report.Observations[id] = node.projectID, node.observedAt
	}
	for _, edge := range edges {
		publicEdges = append(publicEdges, edge)
	}
	sort.SliceStable(publicEdges, func(i, j int) bool {
		if Str(publicEdges[i], "fromRunId") != Str(publicEdges[j], "fromRunId") {
			return numericRunLess(Str(publicEdges[i], "fromRunId"), Str(publicEdges[j], "fromRunId"))
		}
		return numericRunLess(Str(publicEdges[i], "toRunId"), Str(publicEdges[j], "toRunId"))
	})
	cycleEdges := graphCycles(publicNodes, publicEdges, rootID)
	if len(cycleEdges) != 0 {
		report.Limitations = append(report.Limitations, Limitation{Code: "GRAPH_CYCLE_DETECTED", Message: "The inspected execution graph contains a cycle", Source: "dependencies", RunID: rootID})
	}
	report.Graph = Object{"rootRunId": rootID, "complete": unexpanded == 0, "nodes": publicNodes, "edges": publicEdges, "cycles": cycleEdges, "unexpanded": unexpanded}
	report.OmittedTargets = len(omitted)
	return report, nil
}
