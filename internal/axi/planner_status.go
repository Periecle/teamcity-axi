package axi

func revisionMatch(run Object, revision, root string) string {
	if revision == "" || root == "" {
		return "unverified"
	}
	matching := []Object{}
	for _, item := range Objects(run["revisions"]) {
		if Str(item, "vcsRootId") == root {
			matching = append(matching, item)
		}
	}
	if len(matching) != 1 {
		return "unverified"
	}
	if Str(matching[0], "revision") == revision {
		return "exact"
	}
	return "different"
}

// MatchCheckout assesses the newest exact selected-root observation in the bounded window.
func MatchCheckout(ec ExecutionContext, snapshot Object) Object {
	id := ec.Job
	if snapshot != nil && Str(Obj(snapshot["job"]), "id") != "" {
		id = Str(Obj(snapshot["job"]), "id")
	}
	return AssessStatusJob(id, snapshot, ec.Revision, ec.VCSRootID)
}

func AssessStatusJob(id string, snapshot Object, revision, root string) Object {
	availability := "unavailable"
	if snapshot != nil {
		availability = "available"
	}
	page := Obj(snapshot["page"])
	value := Object{"jobId": id, "required": true, "availability": availability, "match": "unverified", "assessment": "unverified", "run": nil, "activity": nil, "candidateCount": Int(page, "providerReturned"), "limitations": []Limitation{}}
	notes := []Limitation{}
	note := func(code, message string) {
		notes = append(notes, Limitation{Code: code, Message: message, Source: "run"})
		value["limitations"] = notes
	}
	if snapshot == nil {
		note("REQUIRED_JOB_UNAVAILABLE", "The required job was not returned; absence and permission coverage are unverified")
		return value
	}
	runs := Objects(page["runs"])
	index, queued, running := -1, 0, 0
	for i, run := range runs {
		if index < 0 && revisionMatch(run, revision, root) == "exact" {
			index = i
		}
		if Str(run, "state") == "queued" {
			queued++
		}
		if Str(run, "state") == "running" {
			running++
		}
	}
	var selected Object
	if index >= 0 {
		selected = runs[index]
	} else if len(runs) != 0 {
		selected = runs[0]
	}
	value["activity"] = Object{"queued": queued, "running": running, "scope": "returnedCandidates"}
	if selected != nil {
		value["run"], value["match"] = selected, revisionMatch(selected, revision, root)
	}
	for _, limitation := range plannerLimitations(page["limitations"]) {
		if limitation.Code == "UNSAFE_CONTINUATION" {
			notes = append(notes, limitation)
		}
	}
	if len(notes) != 0 {
		value["limitations"] = notes
		return value
	}
	if Str(value, "match") != "exact" || selected == nil {
		note("EXACT_REVISION_UNVERIFIED", "No exact selected-root checkout was verified in the bounded candidate window")
		return value
	}
	for _, item := range Objects(selected["revisions"]) {
		if Str(item, "vcsRootId") != root {
			note("OTHER_ROOTS_UNVERIFIED", "The selected root matches; other reported root revisions do not have local checkout verification")
			return value
		}
	}
	personal, specified := selected["personal"].(bool)
	if !specified || personal {
		note("PERSONAL_CHECKOUT_UNVERIFIED", "A personal or unspecified checkout cannot certify the committed worktree")
		return value
	}
	for i := 0; i < index; i++ {
		if revisionMatch(runs[i], revision, root) == "unverified" {
			note("NEWER_REVISION_UNVERIFIED", "A newer candidate lacks selected-root revision evidence")
			return value
		}
	}
	state, result := Str(selected, "state"), Str(selected, "result")
	if state == "queued" || state == "running" {
		value["assessment"] = "in_progress"
	} else if state == "finished" && result == "success" {
		value["assessment"] = "passed"
	} else if state == "finished" && failureResult(result) {
		value["assessment"] = "failed"
	} else {
		note("RUN_OUTCOME_UNVERIFIED", "The selected execution lacks a verified final outcome")
	}
	return value
}
