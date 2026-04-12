package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/writ-fm/go/internal/scheduler"
)

var schedulePath string

var rootCmd = &cobra.Command{
	Use:   "schedule",
	Short: "WRIT-FM schedule tool",
	Long:  "Parse config/schedule.yaml and resolve the active show.",
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the schedule file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := scheduler.LoadSchedule(schedulePath); err != nil {
			return err
		}
		fmt.Println("OK")
		return nil
	},
}

var showsCmd = &cobra.Command{
	Use:   "shows",
	Short: "List all defined shows",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := scheduler.LoadSchedule(schedulePath)
		if err != nil {
			return err
		}
		for id, show := range s.Shows {
			fmt.Printf("  %-25s  host=%-20s  %s\n", id, show.Host, show.Name)
		}
		return nil
	},
}

var nowCmd = &cobra.Command{
	Use:   "now",
	Short: "Print the active show (default: current time)",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := scheduler.LoadSchedule(schedulePath)
		if err != nil {
			return err
		}

		when := time.Now()
		if at, _ := cmd.Flags().GetString("at"); at != "" {
			when, err = time.ParseInLocation("2006-01-02 15:04", at, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --at format (want YYYY-MM-DD HH:MM): %w", err)
			}
		}

		resolved, err := s.Resolve(when)
		if err != nil {
			return err
		}

		dayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
		dayIdx := (int(when.Weekday()) + 6) % 7
		fmt.Printf("%s %s -- %s (%s)\n", dayNames[dayIdx], when.Format("15:04"), resolved.Name, resolved.ShowID)
		fmt.Printf("  Host:     %s\n", resolved.Host)
		fmt.Printf("  Focus:    %s\n", resolved.TopicFocus)
		fmt.Printf("  Segments: %v\n", resolved.SegmentTypes)
		fmt.Printf("  Bumper:   %s\n", resolved.BumperStyle)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&schedulePath, "schedule", defaultSchedulePath(), "Path to schedule.yaml")
	nowCmd.Flags().String("at", "", `Override time: "YYYY-MM-DD HH:MM"`)
	rootCmd.AddCommand(validateCmd, showsCmd, nowCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func defaultSchedulePath() string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "..", "..", "config", "schedule.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Clean(candidate)
		}
	}
	return "config/schedule.yaml"
}
