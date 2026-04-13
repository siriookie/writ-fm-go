package scheduler

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// projectRoot returns the repo root (three levels up from this test file).
func projectRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = …/go/writ-fm-go/internal/scheduler/loader_test.go
	// root      = …/go/writ-fm-go/  (two levels up)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Clean(root)
}

func realSchedulePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "config", "schedule.yaml")
}

// ---- LoadSchedule with the real config/schedule.yaml ----------------------

func TestLoadSchedule_RealFile(t *testing.T) {
	s, err := LoadSchedule(realSchedulePath(t))
	if err != nil {
		t.Fatalf("LoadSchedule returned error: %v", err)
	}
	if len(s.Shows) == 0 {
		t.Error("expected at least one show")
	}
	if len(s.Base) == 0 {
		t.Error("expected at least one base block")
	}
}

func TestLoadSchedule_AllShowsHaveRequiredFields(t *testing.T) {
	s, err := LoadSchedule(realSchedulePath(t))
	if err != nil {
		t.Fatalf("LoadSchedule: %v", err)
	}
	for id, show := range s.Shows {
		if show.Name == "" {
			t.Errorf("show %q: empty Name", id)
		}
		if show.Description == "" {
			t.Errorf("show %q: empty Description", id)
		}
		if show.Host == "" {
			t.Errorf("show %q: empty Host", id)
		}
		if len(show.SegmentTypes) == 0 {
			t.Errorf("show %q: empty SegmentTypes", id)
		}
	}
}

func TestLoadSchedule_OverrideExists(t *testing.T) {
	s, err := LoadSchedule(realSchedulePath(t))
	if err != nil {
		t.Fatalf("LoadSchedule: %v", err)
	}
	if len(s.Overrides) == 0 {
		t.Error("expected at least one override block")
	}
}

func TestLoadSchedule_ResolveCurrentTime(t *testing.T) {
	s, err := LoadSchedule(realSchedulePath(t))
	if err != nil {
		t.Fatalf("LoadSchedule: %v", err)
	}
	resolved, err := s.Resolve(time.Now())
	if err != nil {
		t.Fatalf("Resolve(now): %v", err)
	}
	if resolved.ShowID == "" {
		t.Error("resolved show has empty ShowID")
	}
}

// ---- Error paths -----------------------------------------------------------

func TestLoadSchedule_FileNotFound(t *testing.T) {
	_, err := LoadSchedule("/nonexistent/path/schedule.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadSchedule_InvalidYAML(t *testing.T) {
	f := writeTempYAML(t, "this: is: not: valid: yaml: [\n")
	_, err := LoadSchedule(f)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadSchedule_MissingShows(t *testing.T) {
	yaml := `
schedule:
  base:
    - start: "00:00"
      end: "00:00"
      show: "x"
`
	f := writeTempYAML(t, yaml)
	_, err := LoadSchedule(f)
	if err == nil {
		t.Fatal("expected error for missing shows section")
	}
}

func TestLoadSchedule_MissingBase(t *testing.T) {
	yaml := `
shows:
  x:
    name: "X"
    description: "X"
    host: "liminal_operator"
    bumper_style: "ambient"
    segment_types: ["deep_dive"]
schedule:
  overrides: []
`
	f := writeTempYAML(t, yaml)
	_, err := LoadSchedule(f)
	if err == nil {
		t.Fatal("expected error for missing base")
	}
}

func TestLoadSchedule_InvalidTime(t *testing.T) {
	yaml := `
shows:
  x:
    name: "X"
    description: "X"
    host: "liminal_operator"
    bumper_style: "ambient"
    segment_types: ["deep_dive"]
schedule:
  base:
    - start: "25:00"
      end: "00:00"
      show: "x"
`
	f := writeTempYAML(t, yaml)
	_, err := LoadSchedule(f)
	if err == nil {
		t.Fatal("expected error for invalid time 25:00")
	}
}

func TestLoadSchedule_InvalidDayToken(t *testing.T) {
	yaml := `
shows:
  x:
    name: "X"
    description: "X"
    host: "liminal_operator"
    bumper_style: "ambient"
    segment_types: ["deep_dive"]
schedule:
  base:
    - start: "00:00"
      end: "24:00"
      show: "x"
  overrides:
    - days: ["notaday"]
      start: "12:00"
      end: "14:00"
      show: "x"
`
	f := writeTempYAML(t, yaml)
	_, err := LoadSchedule(f)
	if err == nil {
		t.Fatal("expected error for invalid day token")
	}
}

// ---- parseDays aliases -----------------------------------------------------

func TestParseDays_Aliases(t *testing.T) {
	tests := []struct {
		tokens []string
		count  int
	}{
		{[]string{"daily"}, 7},
		{[]string{"all"}, 7},
		{[]string{"weekday"}, 5},
		{[]string{"weekend"}, 2},
		{[]string{"monday", "friday"}, 2},
	}
	for _, tc := range tests {
		days, err := parseDays(tc.tokens)
		if err != nil {
			t.Errorf("parseDays(%v): unexpected error: %v", tc.tokens, err)
			continue
		}
		if len(days) != tc.count {
			t.Errorf("parseDays(%v): got %d days, want %d", tc.tokens, len(days), tc.count)
		}
	}
}

func TestParseDays_Empty(t *testing.T) {
	_, err := parseDays(nil)
	if err == nil {
		t.Fatal("expected error for empty days")
	}
}

// ---- helper ----------------------------------------------------------------

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "schedule-*.yaml")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	f.Close()
	return f.Name()
}
