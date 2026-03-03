package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

var refreshDataCmd = &cobra.Command{
	Use:   "refresh-data",
	Short: "Reconcile Jira data and compute scores",
	Long: `Fetch current Jira issue data for all configured teams and compute scores.

Runs synchronously, printing progress to stdout. The scoring window covers
the current sprint and the previous sprint, calculated from the configured
reference date and sprint duration (default: 3-week sprints).

Fetched data is stored in memory and available for subsequent scorecard
lookups.

This command validates connectivity and data quality without rendering
a scorecard.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		jiraClient, err := createJiraClient()
		if err != nil {
			return err
		}

		atField, err := resolveActivityTypeField(cfg)
		if err != nil {
			return err
		}

		sprintFetcher := createSprintFetcher()
		rec := reconciler.NewReconciler(jiraClient.Issue, cfg, store, atField, sprintFetcher)

		if sinceFlag != "" {
			since, err := time.Parse("2006-01-02", sinceFlag)
			if err != nil {
				return fmt.Errorf("invalid --since date %q: must be YYYY-MM-DD", sinceFlag)
			}
			rec.SetSinceOverride(since)
		}

		fmt.Println("Starting data refresh...")
		start := time.Now()

		if err := rec.Refresh(context.Background()); err != nil {
			return fmt.Errorf("refresh failed: %w", err)
		}

		state := store.GetState()
		duration := time.Since(start)
		fmt.Printf("Refresh complete: %d issues across all teams (%.1fs)\n",
			state.IssueCount, duration.Seconds())

		return nil
	},
}

func init() {
	rootCmd.AddCommand(refreshDataCmd)
}
