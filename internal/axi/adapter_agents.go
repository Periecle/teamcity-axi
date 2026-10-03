package axi

import "strconv"

const AgentFields = "id,name,connected,enabled,authorized,pool(id,name),build(id,buildTypeId,buildType(id,projectId))"

func ValidateAgentID(value string, pool bool) (err error) {
	defer adapterRecover(&err)
	pattern := exactPositive
	if pool {
		pattern = exactPool
	}
	n, e := strconv.ParseInt(value, 10, 64)
	if !pattern.MatchString(value) || e != nil || n > maxSafeInteger {
		adapterFail("USAGE_ERROR", "Invalid exact numeric agent or pool identity", 2)
	}
	return nil
}
func agentFilters(q ReadRequest) []string {
	scopeIdentities(q, "Invalid agent scope identity")
	filters := []string{}
	if q.PoolID != "" {
		if e := ValidateAgentID(q.PoolID, true); e != nil {
			panic(AsDomainError(e))
		}
		filters = append(filters, "pool:(id:"+q.PoolID+")")
	}
	if q.JobID != "" {
		filters = append(filters, "compatible:(buildType:"+idCondition(q.JobID)+")")
	} else if q.ProjectID != "" {
		filters = append(filters, "compatible:(buildType:(project:"+idCondition(q.ProjectID)+"))")
	}
	return filters
}
func AgentFilters(q ReadRequest) (result []string, err error) {
	defer adapterRecover(&err)
	return agentFilters(q), nil
}
func AgentRequest(q ReadRequest) (result AdapterRequest, err error) {
	defer adapterRecover(&err)
	filters := append(agentFilters(q), "defaultFilter:false")
	if q.JobID == "" && q.ProjectID == "" && q.PoolID == "" {
		adapterFail("CONTEXT_REQUIRED", "Agent list requires a pool, job or project", 2)
	}
	boundedQuery(q, "Invalid bounded agent query")
	return pageRequest("agents", filters, "count,nextHref,agent("+AgentFields+")", q), nil
}
func normalizeAgent(value any, q ReadRequest, secrets []string) (Object, []Limitation) {
	dto := adapterObject(value)
	id := adapterIdentity(dto["id"], true)
	notes := []Limitation{}
	note := func(code, message string) {
		uniqueNote(&notes, Limitation{Code: code, Message: message, Source: "agent"})
	}
	name := adapterString(dto["name"])
	if name == "" {
		adapterInvalid("Invalid safe agent metadata")
	}
	state := func(key string) any {
		if dto[key] == nil {
			note("AGENT_STATUS_UNAVAILABLE", "Some agent availability states were not reported")
			return nil
		}
		return adapterFlag(dto[key])
	}
	var pool any
	if dto["pool"] != nil {
		p := adapterObject(dto["pool"])
		v := p["id"]
		if n, ok := adapterNumber(v); ok {
			if n < 0 {
				adapterInvalid("Invalid safe agent metadata")
			}
			v = strconv.FormatInt(n, 10)
		}
		poolID := adapterIdentity(v, false)
		if e := ValidateAgentID(poolID, true); e != nil {
			adapterInvalid("Invalid safe agent metadata")
		}
		var poolName any
		if p["name"] != nil {
			poolName = SanitizeText(adapterString(p["name"]), secrets)
		}
		pool = Object{"id": poolID, "name": poolName}
	} else {
		note("AGENT_POOL_UNAVAILABLE", "The agent pool was not reported")
	}
	if q.PoolID != "" && (pool == nil || Str(pool.(Object), "id") != q.PoolID) {
		adapterFail("CONTEXT_MISMATCH", "Agent belongs to another selected pool", 1)
	}
	var active any
	activeState := "not_reported"
	if build, present := dto["build"]; present && build == nil {
		activeState = "idle"
	} else if present {
		b := adapterObject(build)
		jobID := adapterIdentity(b["buildTypeId"], false)
		bt := adapterObject(b["buildType"])
		if adapterIdentity(bt["id"], false) != jobID {
			adapterInvalid("Invalid safe agent metadata")
		}
		project := adapterIdentity(bt["projectId"], false)
		active = Object{"id": adapterIdentity(b["id"], true), "jobId": jobID, "projectId": project}
		activeState = "reported"
		if q.ProjectID != "" && project != q.ProjectID {
			active = nil
			activeState = "unavailable"
			note("ACTIVE_RUN_OUTSIDE_SCOPE", "The active execution pointer is outside the selected project")
		}
	} else {
		note("ACTIVE_RUN_UNREPORTED", "The active execution was not reported; idleness is unverified")
	}
	agent := Object{"id": id, "name": SanitizeText(name, secrets), "connected": state("connected"), "enabled": state("enabled"), "authorized": state("authorized"), "pool": pool, "activeRun": active, "activeRunState": activeState}
	return agent, notes
}
func NormalizeAgent(value any, q ReadRequest, secrets []string) (agent Object, notes []Limitation, err error) {
	defer adapterRecover(&err)
	agent, notes = normalizeAgent(value, q, secrets)
	return
}
func NormalizeAgentPage(value any, q ReadRequest, server string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	rows := adapterRows(dto, "agent", q.Count)
	notes := []Limitation{}
	items := []Object{}
	ids := map[string]bool{}
	for _, row := range rows {
		agent, ls := normalizeAgent(row, q, secrets)
		id := Str(agent, "id")
		if ids[id] {
			adapterInvalid("Invalid safe agent metadata")
		}
		ids[id] = true
		for _, n := range ls {
			uniqueNote(&notes, n)
		}
		items = append(items, agent)
	}
	page := adapterPage(items, len(rows), notes)
	r, e := AgentRequest(q)
	if e != nil {
		panic(AsDomainError(e))
	}
	adapterContinuation(dto, page, r, q, server, "agent", "Agent continuation could not be safely reconstructed", "Bounded agent collection exhaustion is unverified")
	return page, nil
}
