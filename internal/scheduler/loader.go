package scheduler

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/writ-fm/go/internal/domain"
)

// dayToIndex maps the three-letter day abbreviation (Mon=0 … Sun=6) to
// an integer index that matches Python's datetime.weekday() convention.
var dayToIndex = map[string]int{
	"mon": 0, "tue": 1, "wed": 2, "thu": 3, "fri": 4, "sat": 5, "sun": 6,
}

// dayAliases maps long-form day names to their three-letter abbreviation.
var dayAliases = map[string]string{
	"monday": "mon", "tuesday": "tue", "wednesday": "wed",
	"thursday": "thu", "friday": "fri", "saturday": "sat", "sunday": "sun",
}

var timeRe = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)

// parseTimeHHMM converts "HH:MM" to minutes-since-midnight.
func parseTimeHHMM(value string) (int, error) {
	value = strings.TrimSpace(value)
	m := timeRe.FindStringSubmatch(value)
	if m == nil {
		return 0, schedErr("invalid time (expected HH:MM): %q", value)
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	if hour > 23 || minute > 59 {
		return 0, schedErr("invalid time (out of range): %q", value)
	}
	return hour*60 + minute, nil
}

// parseDays converts a list of day strings into a set of weekday indices.
// Supports three-letter abbreviations, full names, and aliases: "daily",
// "all", "weekday", "weekend".
func parseDays(raw []string) (map[int]struct{}, error) {
	if len(raw) == 0 {
		return nil, schedErr("missing required field: days")
	}
	days := make(map[int]struct{})
	for _, token := range raw {
		tok := strings.ToLower(strings.TrimSpace(token))
		if alias, ok := dayAliases[tok]; ok {
			tok = alias
		}
		switch tok {
		case "daily", "all":
			for i := 0; i < 7; i++ {
				days[i] = struct{}{}
			}
		case "weekday":
			for i := 0; i < 5; i++ { // Mon–Fri
				days[i] = struct{}{}
			}
		case "weekend":
			days[5] = struct{}{} // Sat
			days[6] = struct{}{} // Sun
		default:
			idx, ok := dayToIndex[tok]
			if !ok {
				return nil, schedErr("invalid day token: %q", tok)
			}
			days[idx] = struct{}{}
		}
	}
	return days, nil
}

// ---- YAML unmarshalling structs --------------------------------------------

type yamlFile struct {
	Timezone string                    `yaml:"timezone"`
	Shows    map[string]yamlShow       `yaml:"shows"`
	Schedule yamlScheduleSection       `yaml:"schedule"`
}

type yamlShow struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Host         string            `yaml:"host"`
	TopicFocus   string            `yaml:"topic_focus"`
	SegmentTypes []string          `yaml:"segment_types"`
	BumperStyle  string            `yaml:"bumper_style"`
	Voices       map[string]string `yaml:"voices"`
}

type yamlScheduleSection struct {
	Base      []yamlBlock `yaml:"base"`
	Overrides []yamlBlock `yaml:"overrides"`
}

type yamlBlock struct {
	Start string   `yaml:"start"`
	End   string   `yaml:"end"`
	Show  string   `yaml:"show"`
	Days  []string `yaml:"days"`
}

// ---- LoadSchedule ----------------------------------------------------------

// LoadSchedule reads path, parses schedule.yaml, and returns a validated
// StationSchedule ready for resolution.
func LoadSchedule(path string) (*StationSchedule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw yamlFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, schedErr("failed to parse schedule YAML: %v", err)
	}

	if len(raw.Shows) == 0 {
		return nil, schedErr("missing or invalid `shows` section")
	}

	shows, err := parseShows(raw.Shows)
	if err != nil {
		return nil, err
	}

	if len(raw.Schedule.Base) == 0 {
		return nil, schedErr("schedule.base must be a non-empty list")
	}

	base, err := parseBlocks(raw.Schedule.Base, false)
	if err != nil {
		return nil, err
	}

	overrides, err := parseBlocks(raw.Schedule.Overrides, true)
	if err != nil {
		return nil, err
	}

	station := &StationSchedule{
		Shows:     shows,
		Base:      base,
		Overrides: overrides,
	}
	if err := station.Validate(); err != nil {
		return nil, err
	}
	return station, nil
}

func parseShows(raw map[string]yamlShow) (map[string]*domain.Show, error) {
	shows := make(map[string]*domain.Show, len(raw))
	for id, cfg := range raw {
		name := strings.TrimSpace(cfg.Name)
		desc := strings.TrimSpace(cfg.Description)
		if name == "" || desc == "" {
			return nil, schedErr("show %q: missing name or description", id)
		}

		host := strings.TrimSpace(cfg.Host)
		if host == "" {
			return nil, schedErr("show %q: missing host", id)
		}
		if len(cfg.SegmentTypes) == 0 {
			return nil, schedErr("show %q: missing segment_types", id)
		}
		segTypes := cfg.SegmentTypes
		bumper := strings.TrimSpace(cfg.BumperStyle)
		if bumper == "" {
			return nil, schedErr("show %q: missing bumper_style", id)
		}
		voices := cfg.Voices
		if voices == nil {
			voices = map[string]string{}
		}

		shows[id] = &domain.Show{
			ShowID:       id,
			Name:         name,
			Description:  desc,
			Host:         host,
			TopicFocus:   cfg.TopicFocus,
			SegmentTypes: segTypes,
			BumperStyle:  bumper,
			Voices:       voices,
		}
	}
	return shows, nil
}

func parseBlocks(raw []yamlBlock, dayAware bool) ([]*domain.ScheduleBlock, error) {
	blocks := make([]*domain.ScheduleBlock, 0, len(raw))
	for _, b := range raw {
		block, err := parseBlock(b, dayAware)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func parseBlock(b yamlBlock, dayAware bool) (*domain.ScheduleBlock, error) {
	start, err := parseTimeHHMM(b.Start)
	if err != nil {
		return nil, err
	}
	end, err := parseTimeHHMM(b.End)
	if err != nil {
		return nil, err
	}
	showID := strings.TrimSpace(b.Show)
	if showID == "" {
		return nil, schedErr("schedule block missing `show`")
	}

	var days map[int]struct{}
	if dayAware {
		days, err = parseDays(b.Days)
		if err != nil {
			return nil, err
		}
	}

	return &domain.ScheduleBlock{
		StartMinute: start,
		EndMinute:   end,
		ShowID:      showID,
		Days:        days,
	}, nil
}
