package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"llm-wiki/internal/scheduler"
)

// ScheduleCommand implements scheduling-related CLI commands.

// NewScheduleCmd creates the schedule command.
func NewScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage scheduled maintenance tasks",
		Long: `Manage automated maintenance tasks for wiki quality control.
Commands available: list, run, add, remove, enable, disable`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewScheduleListCmd())
	cmd.AddCommand(NewScheduleRunCmd())
	cmd.AddCommand(NewScheduleAddCmd())
	cmd.AddCommand(NewScheduleRemoveCmd())
	cmd.AddCommand(NewScheduleEnableCmd())
	cmd.AddCommand(NewScheduleDisableCmd())

	return cmd
}

// NewScheduleListCmd lists all scheduled tasks.
func NewScheduleListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all scheduled tasks",
		Long:  `Show all configured maintenance tasks and their status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat, _ := cmd.Flags().GetString("output")
			manager := createScheduler()
			return listTasks(manager, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&cmdOutputFormat, "output", "o", "text",
		"Output format: text, json")

	return cmd
}

// NewScheduleRunCmd executes tasks immediately.
func NewScheduleRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [task-id]",
		Short: "Execute a scheduled task",
		Long:  `Manually execute a scheduled task or run all pending tasks.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wikiDir := getWikiDir()
			manager := createSchedulerWithDeps(wikiDir)
			
			if len(args) == 0 {
				return runAllTasks(manager)
			}
			return runSingleTask(manager, args[0])
		},
	}

	return cmd
}

// NewScheduleAddCmd adds a new scheduled task.
func NewScheduleAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <type> <frequency>",
		Short: "Add a new scheduled task",
		Long:  `Add a new maintenance task with specified type and frequency.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := createScheduler()
			return addTask(manager, args[0], args[1])
		},
	}

	return cmd
}

// NewScheduleRemoveCmd removes a scheduled task.
func NewScheduleRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <task-id>",
		Short: "Remove a scheduled task",
		Long:  `Delete a scheduled task from the system.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := createScheduler()
			return removeTask(manager, args[0])
		},
	}

	return cmd
}

// NewScheduleEnableCmd enables a scheduled task.
func NewScheduleEnableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <task-id>",
		Short: "Enable a scheduled task",
		Long:  `Activate a disabled maintenance task.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := createScheduler()
			return enableTask(manager, args[0])
		},
	}

	return cmd
}

// NewScheduleDisableCmd disables a scheduled task.
func NewScheduleDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <task-id>",
		Short: "Disable a scheduled task",
		Long:  `Deactivate a maintenance task without removing it.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := createScheduler()
			return disableTask(manager, args[0])
		},
	}

	return cmd
}

// listTasks displays all scheduled tasks.
func listTasks(manager *scheduler.Manager, format string) error {
	tasks := manager.ListTasks()

	fmt.Println("=== Scheduled Tasks ===")
	
	if len(tasks) == 0 {
		fmt.Println("No tasks scheduled. Use 'llm-wiki schedule add' to add one.")
		return nil
	}

	for _, task := range tasks {
		statusIcon := "⏸️"
		if task.Enabled {
			statusIcon = "✅"
		}

		fmt.Printf("%s %-30s %-20s %s\n", 
			statusIcon, 
			task.Name,
			string(task.Schedule),
			formatNextRun(task.NextRun))

		if strings.TrimSpace(task.Description) != "" {
			fmt.Printf("   %s\n", task.Description)
		}
		
		fmt.Printf("   Priority: %d | Created: %s\n\n", 
			task.Priority, 
			task.CreatedAt.Format("2006-01-02"))
	}

	return nil
}
// runAllTasks executes all pending tasks.
func runAllTasks(manager *scheduler.Manager) error {
	ctx := context.Background()
	
	fmt.Println("🔄 Running all pending maintenance tasks...")
	results, err := manager.ExecuteTasks(ctx)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	fmt.Printf("\nCompleted %d tasks:\n\n", len(results))
	
	for _, result := range results {
		icon := "✅"
		if result.Status == scheduler.StatusFailed {
			icon = "❌"
		}
		fmt.Printf("%s %s (%v)\n", icon, result.TaskID, result.Duration)
		if result.Error != "" {
			fmt.Printf("   Error: %s\n", result.Error)
		}
		if result.Metrics != nil && len(result.Metrics) > 0 {
			fmt.Printf("   Metrics:\n")
			for k, v := range result.Metrics {
				fmt.Printf("     - %v: %v\n", k, v)
			}
		}
	}

	return nil
}

// runSingleTask executes a specific task.
func runSingleTask(manager *scheduler.Manager, taskID string) error {
	ctx := context.Background()

	task, err := manager.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	fmt.Printf("🔄 Running task: %s\n", task.Name)
	
	result, err := manager.ExecuteTasks(ctx)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	for _, r := range result {
		if r.TaskID == taskID && r.Status == scheduler.StatusCompleted {
			fmt.Printf("✅ Task completed in %s\n", r.Duration)
			return nil
		}
	}

	fmt.Printf("❌ Task execution pending or failed\n")
	return nil
}

// addTask creates a new scheduled task.
func addTask(manager *scheduler.Manager, taskTypeStr, scheduleStr string) error {
	taskType := scheduler.TaskType(taskTypeStr)
	schedule := scheduler.ScheduleFrequency(scheduleStr)
	
	now := time.Now()
	
	task := &scheduler.Task{
		Name:        fmt.Sprintf("%s - %s", taskTypeStr, scheduleStr),
		Type:        taskType,
		Schedule:    schedule,
		NextRun:     now.Add(time.Hour),
		Status:      scheduler.StatusPending,
		Description: fmt.Sprintf("Scheduled %s task running %s", taskTypeStr, scheduleStr),
		Priority:    5,
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	if err := manager.AddTask(task); err != nil {
		return fmt.Errorf("failed to add task: %w", err)
	}
	
	fmt.Printf("✅ Task added: %s (ID: %s)\n", task.Name, task.ID)
	fmt.Printf("   First run: %s\n", task.NextRun.Format(time.DateTime))
	
	return nil
}

// removeTask deletes a task.
func removeTask(manager *scheduler.Manager, taskID string) error {
	if err := manager.RemoveTask(taskID); err != nil {
		return err
	}
	fmt.Printf("✅ Task removed: %s\n", taskID)
	return nil
}

// enableTask activates a task.
func enableTask(manager *scheduler.Manager, taskID string) error {
	if err := manager.EnableTask(taskID); err != nil {
		return err
	}
	fmt.Printf("✅ Task enabled: %s\n", taskID)
	return nil
}

// disableTask deactivates a task.
func disableTask(manager *scheduler.Manager, taskID string) error {
	if err := manager.DisableTask(taskID); err != nil {
		return err
	}
	fmt.Printf("✅ Task disabled: %s\n", taskID)
	return nil
}

// Helper functions

func formatNextRun(t time.Time) string {
	remaining := time.Until(t)
	if remaining < 0 {
		return "overdue"
	}
	days := int(remaining.Hours() / 24)
	if days == 0 {
		hours := int(remaining.Hours())
		if hours == 0 {
			return "now"
		}
		return fmt.Sprintf("in %dh", hours)
	}
	return fmt.Sprintf("in %dd", days)
}
