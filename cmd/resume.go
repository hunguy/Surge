package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <ID> | --all",
	Short: "Resume a paused or failed download",
	Long:  `Resume a paused or failed download by its ID. Use --all to resume all resumable downloads.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		all, err := cmd.Flags().GetBool("all")
		if err != nil {
			return fmt.Errorf("read --all flag: %w", err)
		}

		if !all && len(args) == 0 {
			return fmt.Errorf("provide a download ID or use --all")
		}
		if all && len(args) != 0 {
			return fmt.Errorf("--all cannot be used with a download ID")
		}

		if err := initializeGlobalState(); err != nil {
			return err
		}

		if all {
			return resumeAllDownloads()
		}

		return ExecuteAPIAction(args[0], "/resume", http.MethodPost, "Resumed download")
	},
}

func isResumableDownloadStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paused", "error", "failed", "interrupted", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

// resumeAllDownloads resumes every resumable download reported by the running
// server. The API only supports resuming one download at a time, so this keeps
// the command compatible with older servers while still making --all useful.
func resumeAllDownloads() error {
	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		return fmt.Errorf("failed to connect to Surge server: %w", err)
	}

	downloads, err := GetRemoteDownloads(baseURL, token)
	if err != nil {
		return fmt.Errorf("failed to list downloads: %w", err)
	}

	ids := make([]string, 0, len(downloads))
	for _, download := range downloads {
		if isResumableDownloadStatus(download.Status) {
			ids = append(ids, download.ID)
		}
	}
	sort.Strings(ids)

	if len(ids) == 0 {
		fmt.Println("No resumable downloads to resume.")
		return nil
	}

	var failures []error
	resumed := 0
	for _, id := range ids {
		resp, err := doAPIRequest(http.MethodPost, baseURL, token, "/resume?id="+url.QueryEscape(id), nil)
		if err != nil {
			failures = append(failures, fmt.Errorf("resume %s: %w", id, err))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			failures = append(failures, fmt.Errorf("resume %s: server returned %s", id, resp.Status))
			_ = resp.Body.Close()
			continue
		}
		if err := resp.Body.Close(); err != nil {
			failures = append(failures, fmt.Errorf("resume %s: close response body: %w", id, err))
			continue
		}
		resumed++
	}

	if len(failures) != 0 {
		return fmt.Errorf("resumed %d of %d downloads: %w", resumed, len(ids), errors.Join(failures...))
	}

	fmt.Printf("Resumed %d downloads.\n", resumed)
	return nil
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().Bool("all", false, "Resume all resumable downloads")
}
