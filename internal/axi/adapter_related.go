package axi

const ChangeFields = "id,version,comment,date,vcsRootInstance(vcs-root-id)"

func RelatedRequest(kind string, q ReadRequest, runFields string) (result AdapterRequest, err error) {
	defer adapterRecover(&err)
	runID := adapterIdentity(q.RunID, true)
	boundedQuery(q, "Invalid bounded related-evidence query")
	resource, filters, fields := "builds", []string{"snapshotDependency:(to:(id:" + runID + "),recursive:false)", "defaultFilter:false"}, "count,nextHref,build("+runFields+")"
	if kind == "changes" {
		resource = "changes"
		filters = []string{"build:(id:" + runID + ")"}
		extra := ""
		if q.Files {
			extra = ",files(count,file(file,changeType))"
		}
		fields = "count,nextHref,change(" + ChangeFields + extra + ")"
	}
	return pageRequest(resource, filters, fields, q), nil
}
func relatedContinuation(dto Object, page Object, r AdapterRequest, q ReadRequest, server, kind string) {
	adapterContinuation(dto, page, r, q, server, kind, "Useful rows retained; unsafe continuation was not followed", "No continuation returned; bounded lookup does not prove collection exhaustion")
	notes := page["limitations"].([]Limitation)
	for i, n := range notes {
		if n.Code == "UNSAFE_CONTINUATION" || n.Code == "SCAN_COVERAGE_UNKNOWN" {
			notes[i].RunID = q.RunID
		}
	}
}
func NormalizeChangePage(value any, q ReadRequest, server string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	rows := adapterRows(dto, "change", q.Count)
	notes := []Limitation{}
	items := []Object{}
	ids := map[string]bool{}
	for _, row := range rows {
		change := adapterObject(row)
		id := adapterIdentity(change["id"], true)
		if ids[id] {
			adapterInvalid("Invalid bounded related-evidence response")
		}
		ids[id] = true
		comment := adapterString(change["comment"])
		version := adapterIdentity(change["version"], false)
		if change["vcsRootInstance"] == nil {
			adapterNote(&notes, "CHANGE_ROOT_UNAVAILABLE", "Change omitted because its VCS-root identity is unavailable", "changes", q.RunID)
			continue
		}
		times := []Limitation{}
		stamp := adapterTimestamp(change["date"], "change", &times)
		for _, n := range times {
			n.Source = "changes"
			n.RunID = q.RunID
			notes = append(notes, n)
		}
		item := Object{"id": id, "version": version, "vcsRootId": adapterIdentity(adapterObject(change["vcsRootInstance"])["vcs-root-id"], false), "message": SanitizeText(comment, secrets), "timestamp": stamp}
		if q.Files {
			if files, present := change["files"]; !present {
				adapterNote(&notes, "CHANGE_FILES_UNAVAILABLE", "Requested changed-file evidence was omitted", "changes", q.RunID)
			} else {
				collection := adapterObject(files)
				entries := adapterArray(collection["file"])
				n, ok := adapterNumber(collection["count"])
				if !ok || int(n) != len(entries) {
					adapterInvalid("Invalid bounded related-evidence response")
				}
				names := []string{}
				limit := len(entries)
				if limit > 100 {
					limit = 100
				}
				for _, entry := range entries[:limit] {
					file := adapterObject(entry)
					name := adapterString(file["file"])
					if name == "" || utf16Length(name) > 4096 {
						adapterInvalid("Invalid bounded related-evidence response")
					}
					name = SanitizeText(name, secrets)
					if len([]rune(name)) > 4096 {
						adapterInvalid("Invalid bounded related-evidence response")
					}
					names = append(names, name)
				}
				item["files"] = names
				item["fileCoverage"] = Object{"returned": len(names), "providerReturned": len(entries), "omitted": len(entries) - len(names)}
				if len(entries) > 100 {
					adapterNote(&notes, "CHANGE_FILE_LIMIT", "Changed-file list reached its hundred-name ceiling", "changes", q.RunID)
				}
			}
		}
		items = append(items, item)
	}
	page := adapterPage(items, len(rows), notes)
	r, e := RelatedRequest("changes", q, "")
	if e != nil {
		panic(AsDomainError(e))
	}
	relatedContinuation(dto, page, r, q, server, "changes")
	return page, nil
}
func NormalizeDependencyPage(value any, q ReadRequest, server, runFields string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	rows := adapterRows(dto, "build", q.Count)
	notes := []Limitation{}
	items := []Object{}
	ids := map[string]bool{}
	for _, row := range rows {
		run, project, ls := normalizeRun(row, server, secrets)
		id := Str(run, "id")
		if ids[id] {
			adapterInvalid("Invalid bounded related-evidence response")
		}
		ids[id] = true
		for _, n := range ls {
			if n.RunID == "" {
				n.RunID = id
			}
			notes = append(notes, n)
		}
		var p any
		if project != "" {
			p = project
		}
		items = append(items, Object{"run": run, "projectId": p})
	}
	page := adapterPage(items, len(rows), notes)
	r, e := RelatedRequest("dependencies", q, runFields)
	if e != nil {
		panic(AsDomainError(e))
	}
	relatedContinuation(dto, page, r, q, server, "dependencies")
	return page, nil
}
func NormalizeDependencyCount(value any, runID string) (result int, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	if adapterIdentity(dto["id"], true) != runID {
		adapterFail("CONTEXT_MISMATCH", "Dependency count belongs to another execution", 1)
	}
	n, ok := adapterNumber(adapterObject(dto["snapshot-dependencies"])["count"])
	if !ok || n < 0 {
		adapterInvalid("Invalid bounded related-evidence response")
	}
	return int(n), nil
}
