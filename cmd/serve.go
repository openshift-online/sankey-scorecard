package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/tiwillia/sankey-scorecard/pkg/db"
	"github.com/tiwillia/sankey-scorecard/pkg/handlers"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

var (
	bindAddress string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the API server",
	Long: `Start the HTTP API server for serving scorecard data.

The server exposes REST endpoints for querying scorecards and initiating
data refreshes. When a --database-url or DATABASE_URL is provided, the
server uses PostgreSQL for persistent storage and can serve data
immediately on startup. Otherwise, in-memory storage is used and a
refresh must be initiated via POST /api/refresh_data.`,
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

		// Determine data store: PostgreSQL if configured, otherwise in-memory.
		var dataStore reconciler.DataStore
		dbURL := databaseURL
		if dbURL == "" {
			dbURL = os.Getenv("DATABASE_URL")
		}
		if dbURL != "" {
			pgStore, err := db.NewPGStore(dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer pgStore.Close()
			dataStore = pgStore
			slog.Info("using PostgreSQL store", "has_data", pgStore.HasData())
		} else {
			dataStore = reconciler.NewReconciliationStore()
			slog.Info("using in-memory store")
		}

		sprintFetcher := createSprintFetcher()
		rec := reconciler.NewReconciler(jiraClient.Issue, cfg, dataStore, atField, sprintFetcher)
		router := handlers.NewRouter(cfg, dataStore, rec)

		server := &http.Server{
			Addr:    bindAddress,
			Handler: router,
		}

		// Channel for shutdown signals
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		// Start server in a goroutine
		errCh := make(chan error, 1)
		go func() {
			fmt.Printf("Listening on %s\n", bindAddress)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()

		// Block until signal or server error
		select {
		case sig := <-stop:
			fmt.Printf("\nReceived %s, shutting down...\n", sig)
		case err := <-errCh:
			return fmt.Errorf("server error: %w", err)
		}

		// Graceful shutdown
		if err := server.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}

		fmt.Println("Server stopped.")
		return nil
	},
}

func init() {
	serveCmd.Flags().StringVar(&bindAddress, "bind-address", ":8080", "Server listen address")
	rootCmd.AddCommand(serveCmd)
}
