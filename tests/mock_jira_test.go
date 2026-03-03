//go:build integration

package tests_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
)

// mockJiraIssue represents a simplified Jira issue for the mock server.
type mockJiraIssue struct {
	Key    string                 `json:"key"`
	Fields map[string]interface{} `json:"fields"`
}

// mockSearchResponse mirrors the Jira search API response format.
type mockSearchResponse struct {
	StartAt    int             `json:"startAt"`
	MaxResults int             `json:"maxResults"`
	Total      int             `json:"total"`
	Issues     []mockJiraIssue `json:"issues"`
}

// mockJiraServer creates a test HTTP server that mimics the Jira REST API.
// It dispatches issues based on the component filter found in the JQL query
// and filters by query type (closed vs in-progress).
type mockJiraServer struct {
	server        *httptest.Server
	issuesByComp  map[string][]mockJiraIssue // component name -> issues
	errorForComp  map[string]bool            // component name -> force error
	rateLimitNext bool                       // if true, return 429 on next request
}

func newMockJiraServer() *mockJiraServer {
	m := &mockJiraServer{
		issuesByComp: make(map[string][]mockJiraIssue),
		errorForComp: make(map[string]bool),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/2/search", m.handleSearch)
	m.server = httptest.NewServer(mux)

	return m
}

func (m *mockJiraServer) URL() string {
	return m.server.URL
}

func (m *mockJiraServer) Close() {
	m.server.Close()
}

func (m *mockJiraServer) SetIssues(component string, issues []mockJiraIssue) {
	m.issuesByComp[component] = issues
}

func (m *mockJiraServer) SetError(component string) {
	m.errorForComp[component] = true
}

func (m *mockJiraServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	// Handle rate limiting simulation
	if m.rateLimitNext {
		m.rateLimitNext = false
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	// Parse the JQL from query string or request body
	jql := r.URL.Query().Get("jql")
	if jql == "" {
		// Try parsing from request body (POST)
		var body struct {
			JQL string `json:"jql"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			jql = body.JQL
		}
	}

	// Determine which component or team field value is being queried
	comp := extractComponent(jql)
	if comp == "" {
		comp = extractTeamFieldValue(jql)
	}

	// Check if we should return an error for this component
	if m.errorForComp[comp] {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"errorMessages": "Simulated error"}) //nolint:errcheck
		return
	}

	allIssues := m.issuesByComp[comp]

	// Filter issues based on JQL type
	var issues []mockJiraIssue
	if isClosedJQL(jql) {
		// For closed JQL, return issues with Done-like statuses
		for _, issue := range allIssues {
			status := extractIssueStatus(issue)
			if status == "Closed" || status == "Done" || status == "Resolved" {
				issues = append(issues, issue)
			}
		}
	} else if isInProgressJQL(jql) {
		// For in-progress JQL, return issues with in-progress-like statuses
		for _, issue := range allIssues {
			status := extractIssueStatus(issue)
			if status == "In Progress" || status == "Code Review" || status == "Review" {
				issues = append(issues, issue)
			}
		}
	} else {
		// Fallback: return all issues
		issues = allIssues
	}

	// Handle pagination
	startAt := 0
	maxResults := 100

	if s := r.URL.Query().Get("startAt"); s != "" {
		var v int
		if _, err := json.Number(s).Int64(); err == nil {
			v = int(mustParseInt(s))
		}
		startAt = v
	}

	// Slice the issues for pagination
	end := startAt + maxResults
	if end > len(issues) {
		end = len(issues)
	}
	var page []mockJiraIssue
	if startAt < len(issues) {
		page = issues[startAt:end]
	}

	resp := mockSearchResponse{
		StartAt:    startAt,
		MaxResults: maxResults,
		Total:      len(issues),
		Issues:     page,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// isClosedJQL returns true if the JQL queries for closed/done issues.
func isClosedJQL(jql string) bool {
	return strings.Contains(jql, "statusCategory = Done")
}

// isInProgressJQL returns true if the JQL queries for in-progress issues.
func isInProgressJQL(jql string) bool {
	return strings.Contains(jql, "status in (")
}

// extractIssueStatus gets the status name from a mock issue.
func extractIssueStatus(issue mockJiraIssue) string {
	if status, ok := issue.Fields["status"].(map[string]interface{}); ok {
		if name, ok := status["name"].(string); ok {
			return name
		}
	}
	return ""
}

// extractComponent parses the component name from a JQL query.
// Expects format like: component in ("comp-alpha")
func extractComponent(jql string) string {
	// Look for: component in ("comp-name")
	idx := strings.Index(jql, `component in ("`)
	if idx == -1 {
		return ""
	}
	rest := jql[idx+len(`component in ("`):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// extractTeamFieldValue parses the team field value from a JQL query.
// Expects format like: "Team" = 5695
func extractTeamFieldValue(jql string) string {
	idx := strings.Index(jql, `"Team" = `)
	if idx == -1 {
		return ""
	}
	rest := jql[idx+len(`"Team" = `):]
	// The value ends at the next space or end of string
	end := strings.IndexAny(rest, " \t")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

func mustParseInt(s string) int64 {
	n, _ := json.Number(s).Int64()
	return n
}

// makeMockIssue creates a mockJiraIssue with the given attributes.
func makeMockIssue(key, project, issueType, activityType, status string, components []string) mockJiraIssue {
	fields := map[string]interface{}{
		"project":   map[string]interface{}{"key": project},
		"issuetype": map[string]interface{}{"name": issueType},
		"status":    map[string]interface{}{"name": status},
		"summary":   "Test issue " + key,
		"updated":   "2026-02-05T10:00:00.000+0000",
		"created":   "2026-01-20T10:00:00.000+0000",
	}

	if len(components) > 0 {
		comps := make([]map[string]interface{}, len(components))
		for i, c := range components {
			comps[i] = map[string]interface{}{"name": c}
		}
		fields["components"] = comps
	}

	if activityType != "" {
		fields["customfield_123"] = map[string]interface{}{
			"value": activityType,
		}
	}

	return mockJiraIssue{
		Key:    key,
		Fields: fields,
	}
}
