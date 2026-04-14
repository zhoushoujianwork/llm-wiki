package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"llm-wiki/internal/feedback"
)

// NewFeedbackCmd creates the feedback command.
func NewFeedbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedback",
		Short: "Collect and manage user feedback",
		Long: `Submit, view, and manage feedback about wiki content quality and accuracy.
Commands available: submit, list, resolve`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewFeedbackSubmitCmd())
	cmd.AddCommand(NewFeedbackListCmd())
	cmd.AddCommand(NewFeedbackResolveCmd())
	cmd.AddCommand(NewFeedbackStatsCmd())

	return cmd
}

// NewFeedbackSubmitCmd allows users to submit feedback.
func NewFeedbackSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit [page-path]",
		Short: "Submit feedback about a wiki page",
		Long:  `Report errors, outdated info, or suggest improvements for a wiki page.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat, _ := cmd.Flags().GetString("output")
			collector := createFeedbackCollector("")
			
			if len(args) == 0 {
				fmt.Println("📝 Submitting anonymous feedback...")
				return submitAnonymousFeedback(collector, outputFormat)
			}
			
			pagePath := args[0]
			fmt.Printf("📝 Submitting feedback for: %s\n", pagePath)
			return submitPageFeedback(collector, pagePath, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&cmdOutputFormat, "output", "o", "text",
		"Output format: text, json")

	return cmd
}

// NewFeedbackListCmd lists all feedback.
func NewFeedbackListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all submitted feedback",
		Long:  `Show all feedback entries, optionally filtered by status or type.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat, _ := cmd.Flags().GetString("output")
			collector := createFeedbackCollector("")
			return listFeedback(collector, outputFormat, context.Background())
		},
	}

	cmd.Flags().StringVarP(&cmdOutputFormat, "output", "o", "text",
		"Output format: text, json")

	return cmd
}

// NewFeedbackResolveCmd marks feedback as resolved.
func NewFeedbackResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <feedback-id>",
		Short: "Mark feedback as resolved",
		Long:  `Update the status of feedback after it has been addressed.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collector := createFeedbackCollector("")
			return resolveFeedback(collector, args[0])
		},
	}

	return cmd
}

// NewFeedbackStatsCmd shows feedback statistics.
func NewFeedbackStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show feedback statistics",
		Long:  `Display aggregated statistics about submitted feedback.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			collector := createFeedbackCollector("")
			return showStats(collector)
		},
	}

	return cmd
}

// submitAnonymousFeedback collects feedback interactively.
func submitAnonymousFeedback(c *feedback.CollectorImpl, format string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n--- Submit Anonymous Feedback ---")
	
	fmt.Print("Enter feedback type (error/outdated/incomplete/unclear/suggestion/duplicate/other): ")
	fbTypeStr, _ := reader.ReadString('\n')
	fbTypeStr = strings.TrimSpace(fbTypeStr)
	
	fmt.Print("Describe the issue: ")
	description, _ := reader.ReadString('\n')
	description = strings.TrimSpace(description)

	fmt.Print("Priority (1-5, default 3): ")
	priorityStr, _ := reader.ReadString('\n')
	priorityStr = strings.TrimSpace(priorityStr)
	
	var priority int
	if priorityStr != "" {
		fmt.Sscanf(priorityStr, "%d", &priority)
	}
	if priority < 1 || priority > 5 {
		priority = 3
	}

	fb := &feedback.Feedback{
		Type:        feedback.FeedbackType(fbTypeStr),
		Description: description,
		Priority:    priority,
		Status:      feedback.StatusNew,
		Source:      "user_submission",
	}

	if err := c.SubmitFeedback(context.Background(), fb); err != nil {
		return fmt.Errorf("failed to submit feedback: %w", err)
	}

	fmt.Printf("\n✅ Feedback submitted! ID: %s\n", fb.ID)
	fmt.Printf("   Status: %s\n", fb.Status)
	fmt.Printf("   Priority: %d/5\n", fb.Priority)

	return nil
}

// submitPageFeedback submits feedback for a specific page.
func submitPageFeedback(c *feedback.CollectorImpl, pagePath string, format string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n--- Page Feedback ---")
	
	fmt.Print("Feedback type (error/outdated/incomplete/unclear/broken_link/suggestion/duplicate/other): ")
	fbTypeStr, _ := reader.ReadString('\n')
	fbTypeStr = strings.TrimSpace(fbTypeStr)

	fmt.Print("Description: ")
	description, _ := reader.ReadString('\n')
	description = strings.TrimSpace(description)

	fmt.Print("Priority (1-5, default 3): ")
	priorityStr, _ := reader.ReadString('\n')
	priorityStr = strings.TrimSpace(priorityStr)
	
	var priority int
	if priorityStr != "" {
		fmt.Sscanf(priorityStr, "%d", &priority)
	}
	if priority < 1 || priority > 5 {
		priority = 3
	}

	fb := &feedback.Feedback{
		PagePath:    pagePath,
		Type:        feedback.FeedbackType(fbTypeStr),
		Description: description,
		Priority:    priority,
		Status:      feedback.StatusNew,
		Source:      "user_submission",
		CreatedAt:   time.Now(),
	}

	if err := c.SubmitFeedback(context.Background(), fb); err != nil {
		return fmt.Errorf("failed to submit feedback: %w", err)
	}

	fmt.Printf("\n✅ Feedback submitted for '%s'!\n", pagePath)
	fmt.Printf("   ID: %s | Status: %s | Priority: %d/5\n", fb.ID, fb.Status, fb.Priority)

	return nil
}

// listFeedback shows all feedback entries.
func listFeedback(c *feedback.CollectorImpl, format string, ctx context.Context) error {
	ctx = context.Background()
	fbList, err := c.ListFeedback(ctx, &feedback.FeedbackFilters{})
	if err != nil {
		return fmt.Errorf("failed to list feedback: %w", err)
	}

	fmt.Println("=== Submitted Feedback ===")
	
	if len(fbList) == 0 {
		fmt.Println("No feedback found.")
		return nil
	}

	for _, fb := range fbList {
		statusIcon := "?"
		switch fb.Status {
		case feedback.StatusNew:
			statusIcon = "🆕"
		case feedback.StatusInReview:
			statusIcon = "🔍"
		case feedback.StatusResolved:
			statusIcon = "✅"
		case feedback.StatusRejected:
			statusIcon = "❌"
		}

		typeLabel := string(fb.Type)
		
		fmt.Printf("%s [%s] %-4s Priority: %d/%d\n", 
			statusIcon, fb.Title, typeLabel, fb.Priority, 5)
		fmt.Printf("   %s\n", truncateString(fb.Description, 80))
		fmt.Printf("   ID: %s | Created: %s\n\n", 
			fb.ID[:min(len(fb.ID), 12)]+"...", 
			fb.CreatedAt.Format("2006-01-02"))
	}

	fmt.Printf("Total: %d entries\n", len(fbList))
	return nil
}

// resolveFeedback updates feedback status.
func resolveFeedback(c *feedback.CollectorImpl, feedbackID string) error {
	_, err := c.GetFeedback(feedbackID)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	
	fmt.Print("Resolution (what was done to fix this?): ")
	resolution, _ := reader.ReadString('\n')
	resolution = strings.TrimSpace(resolution)

	if err := c.UpdateStatus(feedbackID, feedback.StatusResolved, resolution); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	fmt.Printf("✅ Feedback %s marked as resolved.\n", feedbackID[:min(len(feedbackID), 12)]+"...")

	return nil
}

// showStats displays feedback statistics.
func showStats(c *feedback.CollectorImpl) error {
	ctx := context.Background()
	stats, err := c.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	fmt.Println("=== Feedback Statistics ===")
	fmt.Printf("Total Feedback: %d\n\n", stats.Total)
	
	fmt.Println("By Type:")
	for fbt, count := range stats.ByType {
		fmt.Printf("  %-15s: %d\n", fbt, count)
	}
	
	fmt.Println("\nBy Status:")
	for fbs, count := range stats.ByStatus {
		fmt.Printf("  %-15s: %d\n", fbs, count)
	}
	
	fmt.Printf("\nAverage Priority: %.2f/5\n", stats.AvgPriority)
	fmt.Printf("Resolution Rate: %.1f%%\n", stats.ResolutionRate*100)
	fmt.Printf("Most Recent: %s\n", stats.MostRecent.Format(time.DateTime))

	return nil
}

// Helper functions

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
