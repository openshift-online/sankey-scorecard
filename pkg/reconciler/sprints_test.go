package reconciler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

var _ = Describe("Sprint Discovery", func() {
	Describe("AgileSprintFetcher", func() {
		It("fetches sprints from the Agile API", func() {
			sprints := []reconciler.SprintInfo{
				{ID: 101, Name: "Sprint 1", State: "closed"},
				{ID: 102, Name: "Sprint 2", State: "active"},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/rest/agile/1.0/board/42/sprint"))
				Expect(r.URL.Query().Get("state")).To(Equal("active,closed"))

				resp := map[string]interface{}{
					"maxResults": 50,
					"startAt":    0,
					"isLast":     true,
					"values":     sprints,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp) //nolint:errcheck
			}))
			defer server.Close()

			fetcher := &reconciler.AgileSprintFetcher{
				BaseURL:    server.URL,
				HTTPClient: server.Client(),
			}

			result, err := fetcher.FetchBoardSprints(context.Background(), 42)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result[0].ID).To(Equal(101))
			Expect(result[0].State).To(Equal("closed"))
			Expect(result[1].ID).To(Equal(102))
			Expect(result[1].State).To(Equal("active"))
		})

		It("returns error on non-200 status", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			fetcher := &reconciler.AgileSprintFetcher{
				BaseURL:    server.URL,
				HTTPClient: server.Client(),
			}

			_, err := fetcher.FetchBoardSprints(context.Background(), 999)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected status 404"))
		})

		It("returns error on invalid JSON response", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("not json")) //nolint:errcheck
			}))
			defer server.Close()

			fetcher := &reconciler.AgileSprintFetcher{
				BaseURL:    server.URL,
				HTTPClient: server.Client(),
			}

			_, err := fetcher.FetchBoardSprints(context.Background(), 42)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to decode"))
		})
	})

	Describe("NoOpSprintFetcher", func() {
		It("returns nil sprints", func() {
			fetcher := &reconciler.NoOpSprintFetcher{}
			result, err := fetcher.FetchBoardSprints(context.Background(), 42)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})
})
