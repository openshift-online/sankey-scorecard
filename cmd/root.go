package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	jira "github.com/andygrunwald/go-jira/v2/onpremise"
	"github.com/spf13/cobra"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
	"github.com/tiwillia/sankey-scorecard/pkg/scorecard"
)

var (
	cfgFile      string
	outputFormat string
	startDate    string
	endDate      string
	statusFilter string
)

// Shared Jira flags (persistent, available to all subcommands)
var (
	jiraURL           string
	jiraAPIToken      string
	activityTypeField string
	sinceFlag         string
	databaseURL       string
)

// Shared state for the CLI process
var (
	store reconciler.DataStore
)

func init() {
	store = reconciler.NewReconciliationStore()
}

var rootCmd = &cobra.Command{
	Use:   "sankey-scorecard [identifier] [flags]",
	Short: "Evaluate teams on Sankey planning framework adherence",
	Long: `Evaluate teams on their Sankey planning framework adherence by analyzing
Jira issue data and producing scorecard reports.

When called with an identifier, displays the scorecard for that entity.
Identifiers are globally unique names. If ambiguous, use a slash-delimited
path to disambiguate (e.g., rosa/aurora or hcm/rosa/aurora).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		identifier := args[0]
		entity, err := cfg.Resolve(identifier)
		if err != nil {
			if isNotFound(err) {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(2)
			}
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}

		filter, err := buildFilterOptions()
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

		teams := entity.Teams()
		rec := reconciler.NewReconciler(jiraClient.Issue, cfg, store, atField, sprintFetcher)

		if sinceFlag != "" {
			since, err := time.Parse("2006-01-02", sinceFlag)
			if err != nil {
				return fmt.Errorf("invalid --since date %q: must be YYYY-MM-DD", sinceFlag)
			}
			rec.SetSinceOverride(since)
		}

		fmt.Fprintf(os.Stderr, "Fetching data for %d team(s)...\n", len(teams))
		start := time.Now()

		if err := rec.RefreshTeams(context.Background(), teams); err != nil {
			return fmt.Errorf("refresh failed: %w", err)
		}

		state := store.GetState()
		duration := time.Since(start)
		fmt.Fprintf(os.Stderr, "Fetched %d issues (%.1fs)\n", state.IssueCount, duration.Seconds())

		if filter.StartDate != nil || filter.EndDate != nil || filter.IssueStatus != "" {
			parts := []string{}
			if filter.IssueStatus != "" {
				parts = append(parts, "status="+filter.IssueStatus)
			}
			if filter.StartDate != nil {
				parts = append(parts, "from="+filter.StartDate.Format("2006-01-02"))
			}
			if filter.EndDate != nil {
				parts = append(parts, "to="+filter.EndDate.Format("2006-01-02"))
			}
			fmt.Fprintf(os.Stderr, "Applying filters: %s\n", strings.Join(parts, ", "))
		}

		fs := scorecard.ComputeFullScorecard(cfg, store, filter)
		return renderEntity(fs, entity)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func initRootFlags() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "Path to config file (default: $RESOURCE_MAP_PATH or /etc/sankey-scorecard/sankey-scorecard.yaml)")
	rootCmd.PersistentFlags().StringVar(&jiraURL, "jira-url", "", "Jira instance URL (required)")
	rootCmd.PersistentFlags().StringVar(&jiraAPIToken, "jira-api-token", "", "Jira API token for authentication (default: $JIRA_API_TOKEN)")
	rootCmd.PersistentFlags().StringVar(&activityTypeField, "activity-type-field", "", "Jira custom field ID for Activity Type (overrides resource map value)")
	rootCmd.PersistentFlags().StringVar(&sinceFlag, "since", "", "Override sprint calendar; include issues updated since this date (YYYY-MM-DD)")
	rootCmd.PersistentFlags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection string for persistent storage (default: in-memory; env: DATABASE_URL)")
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "plaintext", "Output format: plaintext, json, yaml")
	rootCmd.Flags().StringVar(&startDate, "start-date", "", "Include only periods overlapping this start date (YYYY-MM-DD, closed issues only)")
	rootCmd.Flags().StringVar(&endDate, "end-date", "", "Include only periods overlapping this end date (YYYY-MM-DD, closed issues only)")
	rootCmd.Flags().StringVar(&statusFilter, "status", "", "Filter by issue status: closed, in_progress (default: both)")
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

func init() {
	initRootFlags()
}

// createJiraClient validates the required Jira flags and returns a configured client.
func createJiraClient() (*jira.Client, error) {
	if jiraURL == "" {
		return nil, fmt.Errorf("--jira-url is required")
	}
	if jiraAPIToken == "" {
		jiraAPIToken = os.Getenv("JIRA_API_TOKEN")
	}
	if jiraAPIToken == "" {
		return nil, fmt.Errorf("--jira-api-token or JIRA_API_TOKEN environment variable is required")
	}

	tp := &jira.PATAuthTransport{Token: jiraAPIToken}
	httpClient := &http.Client{Transport: tp}
	client, err := jira.NewClient(jiraURL, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}
	return client, nil
}

// resolveActivityTypeField returns the activity type field ID, using the CLI
// flag if set, otherwise falling back to the resource map config value.
func resolveActivityTypeField(cfg *config.ResourceMap) (string, error) {
	if activityTypeField != "" {
		return activityTypeField, nil
	}
	if cfg.Jira.ActivityTypeField != "" {
		return cfg.Jira.ActivityTypeField, nil
	}
	return "", fmt.Errorf("activity type field ID is required: set activity_type_field in the resource map or pass --activity-type-field")
}

// createSprintFetcher creates a BoardSprintFetcher using the same auth as the Jira client.
func createSprintFetcher() reconciler.BoardSprintFetcher {
	if jiraURL == "" || jiraAPIToken == "" {
		return &reconciler.NoOpSprintFetcher{}
	}
	tp := &jira.PATAuthTransport{Token: jiraAPIToken}
	return &reconciler.AgileSprintFetcher{
		BaseURL:    jiraURL,
		HTTPClient: &http.Client{Transport: tp},
	}
}

const defaultConfigPath = "/etc/sankey-scorecard/sankey-scorecard.yaml"

func loadConfig() (*config.ResourceMap, error) {
	if cfgFile != "" {
		return config.LoadFromFile(cfgFile)
	}
	if envPath := os.Getenv("RESOURCE_MAP_PATH"); envPath != "" {
		return config.LoadFromFile(envPath)
	}
	if _, err := os.Stat(defaultConfigPath); err == nil {
		return config.LoadFromFile(defaultConfigPath)
	}
	return nil, fmt.Errorf(
		"no config file found; provide one via:\n" +
			"  --config / -c <path>\n" +
			"  RESOURCE_MAP_PATH=<path>\n" +
			"  " + defaultConfigPath + " (default container path)",
	)
}

func buildFilterOptions() (scorecard.FilterOptions, error) {
	var filter scorecard.FilterOptions

	if startDate != "" {
		t, err := time.Parse("2006-01-02", startDate)
		if err != nil {
			return filter, fmt.Errorf("invalid --start-date %q: must be YYYY-MM-DD", startDate)
		}
		filter.StartDate = &t
	}

	if endDate != "" {
		t, err := time.Parse("2006-01-02", endDate)
		if err != nil {
			return filter, fmt.Errorf("invalid --end-date %q: must be YYYY-MM-DD", endDate)
		}
		filter.EndDate = &t
	}

	if statusFilter != "" {
		if statusFilter != "closed" && statusFilter != "in_progress" {
			return filter, fmt.Errorf("invalid --status %q: must be \"closed\" or \"in_progress\"", statusFilter)
		}
		filter.IssueStatus = statusFilter
	}

	return filter, nil
}

func isNotFound(err error) bool {
	return err != nil && strings.HasSuffix(err.Error(), "not found")
}

func renderEntity(fs scorecard.FullScorecard, entity *config.ResolvedEntity) error {
	var output interface{}

	switch entity.Level {
	case config.LevelOrganization:
		for _, org := range fs.Organizations {
			if org.Identifier == entity.Organization.Identifier {
				output = org
				break
			}
		}
	case config.LevelPillar:
		for _, org := range fs.Organizations {
			if org.Identifier == entity.Organization.Identifier {
				for _, pillar := range org.Pillars {
					if pillar.Identifier == entity.Pillar.Identifier {
						output = pillar
						break
					}
				}
			}
		}
	case config.LevelTeam:
		for _, org := range fs.Organizations {
			if org.Identifier == entity.Organization.Identifier {
				for _, pillar := range org.Pillars {
					if pillar.Identifier == entity.Pillar.Identifier {
						for _, team := range pillar.Teams {
							if team.Identifier == entity.Team.Identifier {
								output = team
								break
							}
						}
					}
				}
			}
		}
	}

	if output == nil {
		return fmt.Errorf("entity not found in scorecard")
	}

	switch outputFormat {
	case "json":
		data, err := scorecard.ToJSON(output)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "yaml":
		data, err := scorecard.ToYAML(output)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
	default:
		fmt.Print(scorecard.ToPlaintext(output))
	}

	return nil
}
