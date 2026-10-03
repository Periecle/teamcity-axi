package testfixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func treeNode(id int) Object {
	o := mockRun(id)
	job := "Payments_Build"
	if id != 482193 {
		job = fmt.Sprintf("Payments_Job_%d", id)
	}
	o["buildTypeId"] = job
	o["buildType"] = Object{"id": job, "name": "Graph fixture", "projectId": "Payments"}
	return o
}

func (m *Server) treeServe(w http.ResponseWriter, r *http.Request, path string, q url.Values, mode string) bool {
	send := func(value any, status int) {
		w.WriteHeader(status)
		if status < 400 && m.respectFields {
			value = SelectFields(value, q.Get("fields"))
		}
		json.NewEncoder(w).Encode(value)
	}
	adjacency := map[int][]int{482193: {482190, 482191}, 482190: {482188}, 482191: {482188}, 482188: {}}
	switch mode {
	case "cycle":
		adjacency = map[int][]int{482193: {482190}, 482190: {482191}, 482191: {482193}}
	case "changed", "changed-metadata", "provisional", "final-unavailable":
		adjacency = map[int][]int{482193: {}}
	case "wide":
		rows := []int{}
		for i := 0; i < 40; i++ {
			rows = append(rows, 482100+i)
		}
		adjacency = map[int][]int{482193: rows}
	}
	match := regexp.MustCompile(`^/app/rest/builds/id:(\d+)$`).FindStringSubmatch(path)
	if match != nil {
		id, _ := strconv.Atoi(match[1])
		if mode == "denied" && id == 482190 {
			send(Object{"message": "Denied child"}, 403)
			return true
		}
		if q.Get("fields") == "id,snapshot-dependencies(count)" {
			count := len(adjacency[id])
			if mode == "mismatch" && id == 482193 {
				count++
			}
			send(Object{"id": id, "snapshot-dependencies": Object{"count": count}}, 200)
			return true
		}
		if id != 482193 {
			send(Object{"message": "Unexpected detail traversal"}, 400)
			return true
		}
		m.mu.Lock()
		m.detailReads++
		n := m.detailReads
		m.mu.Unlock()
		if mode == "final-unavailable" && n > 1 {
			send(Object{"message": "Final read denied"}, 403)
			return true
		}
		value := treeNode(id)
		if mode == "success" {
			value["status"] = "SUCCESS"
		}
		if containsString([]string{"changed", "changed-metadata", "provisional", "final-unavailable"}, mode) {
			value["state"] = "running"
			if mode == "changed" && n > 1 {
				value["state"] = "finished"
				value["status"] = "SUCCESS"
			}
			if mode == "changed-metadata" && n > 1 {
				value["branchName"] = "changed-branch"
			}
		}
		send(value, 200)
		return true
	}
	if path == "/app/rest/problemOccurrences" || path == "/app/rest/testOccurrences" {
		match := regexp.MustCompile(`build:\(id:(\d+)\)`).FindStringSubmatch(q.Get("locator"))
		if match == nil {
			send(Object{"message": "Missing source identity"}, 400)
			return true
		}
		id, _ := strconv.Atoi(match[1])
		problem := strings.HasSuffix(path, "problemOccurrences")
		if mode == "sources-denied" && problem {
			send(Object{"message": "Independent source denied"}, 403)
			return true
		}
		if mode == "logs-needed" && !problem {
			send(Object{"message": "Independent tests unavailable"}, 404)
			return true
		}
		if problem {
			details := "Explicit synthetic problem"
			if mode == "logs-needed" || mode == "log-unicode" {
				details = ""
			}
			send(Object{"count": 1, "problemOccurrence": []Object{{"id": fmt.Sprintf("build:(id:%d),problem:(id:1)", id), "build": Object{"id": id}, "type": "SYNTHETIC", "identity": "synthetic", "details": details}}}, 200)
			return true
		}
		if mode == "log-unicode" {
			send(Object{"count": 0, "testOccurrence": []Object{}}, 200)
			return true
		}
		muted := strings.Contains(q.Get("locator"), "muted:true")
		items := []Object{}
		for number := 1; number <= 2; number++ {
			occurrence := number
			if muted {
				occurrence += 2
			}
			details := "Connection refused"
			if mode == "secret" {
				details = m.token + strings.Repeat("x", 3000)
			}
			items = append(items, Object{"id": fmt.Sprintf("build:(id:%d),id:%d", id, occurrence), "build": Object{"id": id}, "name": "same name", "status": "FAILURE", "muted": muted, "ignored": false, "duration": 25, "details": details, "test": Object{"id": "517450581327024597"}})
		}
		send(Object{"count": 2, "testOccurrence": items}, 200)
		return true
	}
	if path == "/app/messages" {
		if mode == "logs-needed" {
			send(Object{"message": "Log capability unavailable"}, 404)
			return true
		}
		text := "Connection refused"
		if mode == "log-unicode" {
			text = "--error=Connection refused " + strings.Repeat("x", 52) + "🦊"
		}
		send(Object{"messages": []Object{{"id": 12, "text": text, "level": 0, "status": 4, "timestamp": "2026-10-02T00:00:00Z"}}, "lastMessageIndex": 12, "focusIndex": 12, "lastMessageIncluded": true}, 200)
		return true
	}
	if path == "/app/rest/changes" {
		rows := []Object{}
		if mode == "changes-positive" {
			row := mockChange(0, false)
			row["comment"] = "Synthetic contextual change\nFurther detail"
			rows = append(rows, row)
		}
		if mode == "changes-wide" {
			for i := 0; i < 10; i++ {
				row := mockChange(i, false)
				row["comment"] = strings.Repeat("🦊", 1500)
				rows = append(rows, row)
			}
		}
		send(Object{"count": len(rows), "change": rows}, 200)
		return true
	}
	if path == "/app/rest/builds" {
		locator := q.Get("locator")
		match := regexp.MustCompile(`snapshotDependency:\(to:\(id:(\d+)\),recursive:false\)`).FindStringSubmatch(locator)
		if match == nil || !strings.Contains(locator, "defaultFilter:false") {
			send(Object{"message": "Expected immediate dependency locator"}, 400)
			return true
		}
		parent, _ := strconv.Atoi(match[1])
		if mode == "denied" && parent == 482190 {
			send(Object{"message": "Denied child"}, 403)
			return true
		}
		builds := []Object{}
		for _, id := range adjacency[parent] {
			row := treeNode(id)
			if mode == "foreign" && id == 482191 {
				row["state"] = "future"
				delete(row, "revisions")
				Obj(row["buildType"])["projectId"] = "Foreign"
			}
			if mode == "missing-metadata" {
				delete(row, "revisions")
			}
			if mode == "invalid-timestamp" {
				row["startDate"] = "bad"
			}
			builds = append(builds, row)
		}
		value := Object{"count": len(builds), "build": builds}
		if mode == "unsafe" && parent == 482193 {
			value["nextHref"] = "https://attacker.invalid/app/rest/builds"
		}
		if mode == "empty" && parent == 482193 && mockBound(locator, "start", 0) == 0 {
			value["count"] = 0
			value["build"] = []Object{}
			value["nextHref"] = mockNext(path, q, 100, 0)
		}
		send(value, 200)
		return true
	}
	return false
}

type fieldNode struct {
	name     string
	children []fieldNode
}

// SelectFields reproduces TeamCity's nested field selection for fixture DTOs.
func SelectFields(value any, fields string) any {
	if fields == "" {
		return value
	}
	nodes := parseFieldNodes(fields)
	return projectFields(value, nodes)
}

func SelectFieldsChecked(value any, fields string) (any, error) {
	depth := 0
	for _, r := range fields {
		if r == '(' {
			depth++
		}
		if r == ')' {
			depth--
			if depth < 0 {
				return nil, errors.New("unbalanced field projection")
			}
		}
	}
	if depth != 0 {
		return nil, errors.New("unbalanced field projection")
	}
	return SelectFields(value, fields), nil
}

func parseFieldNodes(fields string) []fieldNode {
	parts := []string{}
	start, depth := 0, 0
	for i, r := range fields {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, fields[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, fields[start:])
	nodes := []fieldNode{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := part
		var children []fieldNode
		if opening := strings.Index(part, "("); opening >= 0 {
			name = part[:opening]
			inner := part[opening+1 : len(part)-1]
			if strings.HasPrefix(inner, "$locator:") {
				depth := 0
				split := -1
				for i, r := range inner {
					switch r {
					case '(':
						depth++
					case ')':
						depth--
					case ',':
						if depth == 0 {
							split = i
						}
					}
					if split >= 0 {
						break
					}
				}
				if split >= 0 {
					inner = inner[split+1:]
				} else {
					inner = ""
				}
			}
			children = parseFieldNodes(inner)
		}
		nodes = append(nodes, fieldNode{name, children})
	}
	return nodes
}

func projectFields(value any, nodes []fieldNode) any {
	if rows := Objects(value); rows != nil {
		result := []Object{}
		for _, row := range rows {
			result = append(result, Obj(projectFields(row, nodes)))
		}
		return result
	}
	object := Obj(value)
	if object == nil {
		return value
	}
	result := Object{}
	for _, node := range nodes {
		if node.name == "*" {
			for k, v := range object {
				result[k] = v
			}
			continue
		}
		v, ok := object[node.name]
		if !ok {
			continue
		}
		if len(node.children) > 0 {
			v = projectFields(v, node.children)
		}
		result[node.name] = v
	}
	return result
}
