package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SprintInfo represents a sprint returned from the Jira Agile API.
type SprintInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"` // "active", "closed", "future"
}

// BoardSprintFetcher retrieves sprints for a given board from the Jira Agile API.
type BoardSprintFetcher interface {
	FetchBoardSprints(ctx context.Context, boardID int) ([]SprintInfo, error)
}

// AgileSprintFetcher implements BoardSprintFetcher using the Jira Agile REST API.
type AgileSprintFetcher struct {
	BaseURL    string
	HTTPClient *http.Client
}

// sprintResponse represents the paginated response from the Agile sprint endpoint.
type sprintResponse struct {
	MaxResults int          `json:"maxResults"`
	StartAt    int          `json:"startAt"`
	IsLast     bool         `json:"isLast"`
	Values     []SprintInfo `json:"values"`
}

// FetchBoardSprints retrieves active and closed sprints for a board.
func (f *AgileSprintFetcher) FetchBoardSprints(ctx context.Context, boardID int) ([]SprintInfo, error) {
	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=active,closed", f.BaseURL, boardID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for board %d sprints: %w", boardID, err)
	}

	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sprints for board %d: %w", boardID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching sprints for board %d", resp.StatusCode, boardID)
	}

	var result sprintResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode sprint response for board %d: %w", boardID, err)
	}

	return result.Values, nil
}

// NoOpSprintFetcher is a BoardSprintFetcher that returns no sprints.
// Used when no sprint_board teams are configured.
type NoOpSprintFetcher struct{}

// FetchBoardSprints always returns nil for NoOpSprintFetcher.
func (f *NoOpSprintFetcher) FetchBoardSprints(ctx context.Context, boardID int) ([]SprintInfo, error) {
	return nil, nil
}
