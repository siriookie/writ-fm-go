package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// GET /now-playing  (also GET /)
// ---------------------------------------------------------------------------

func (s *Server) handleNowPlaying(w http.ResponseWriter, _ *http.Request) {
	track := s.state.Get()
	listeners := s.lc.get()

	// Inject live listener count.
	track.Listeners = listeners

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	writeJSON(w, http.StatusOK, track)
}

// ---------------------------------------------------------------------------
// GET /health
// ---------------------------------------------------------------------------

type healthResponse struct {
	Status     string                     `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Components map[string]componentStatus `json:"components"`
	UptimeSecs int64                      `json:"uptime_seconds"`
}

type componentStatus struct {
	Status string `json:"status"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	icecastOK := s.lc.counter != nil
	if icecastOK {
		_, err := s.lc.counter.Listeners(s.cfg.Mount)
		icecastOK = err == nil
	}

	status := "healthy"
	if !icecastOK {
		status = "degraded"
	}

	resp := healthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Components: map[string]componentStatus{
			"icecast": {upDown(icecastOK)},
			"api":     {"up"},
		},
		UptimeSecs: int64(time.Since(s.startedAt).Seconds()),
	}
	writeJSON(w, http.StatusOK, resp)
}

func upDown(ok bool) string {
	if ok {
		return "up"
	}
	return "down"
}

// ---------------------------------------------------------------------------
// GET /schedule
// ---------------------------------------------------------------------------

type scheduleResponse struct {
	Current   *showSummary   `json:"current"`
	Upcoming  []upcomingShow `json:"upcoming"`
	Timestamp time.Time      `json:"timestamp"`
}

type showSummary struct {
	ShowID       string   `json:"show_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Host         string   `json:"host"`
	TopicFocus   string   `json:"topic_focus"`
	SegmentTypes []string `json:"segment_types"`
	BumperStyle  string   `json:"bumper_style"`
}

type upcomingShow struct {
	ShowID       string `json:"show_id"`
	Name         string `json:"name"`
	Host         string `json:"host"`
	TopicFocus   string `json:"topic_focus"`
	StartsAround string `json:"starts_around"`
}

func (s *Server) handleSchedule(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	current, err := s.sched.Resolve(now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := scheduleResponse{
		Current: &showSummary{
			ShowID:       current.ShowID,
			Name:         current.Name,
			Description:  current.Description,
			Host:         current.Host,
			TopicFocus:   current.TopicFocus,
			SegmentTypes: current.SegmentTypes,
			BumperStyle:  current.BumperStyle,
		},
		Upcoming:  buildUpcoming(s.sched, now),
		Timestamp: now,
	}
	writeJSON(w, http.StatusOK, resp)
}

// buildUpcoming samples the next 4 hours every 30 minutes and deduplicates
// consecutive show_ids, returning at most 4 upcoming entries.
func buildUpcoming(sched ScheduleResolver, now time.Time) []upcomingShow {
	var upcoming []upcomingShow
	for minutesAhead := 30; minutesAhead <= 240; minutesAhead += 30 {
		future := now.Add(time.Duration(minutesAhead) * time.Minute)
		show, err := sched.Resolve(future)
		if err != nil {
			continue
		}
		if len(upcoming) > 0 && upcoming[len(upcoming)-1].ShowID == show.ShowID {
			continue // deduplicate consecutive same show
		}
		upcoming = append(upcoming, upcomingShow{
			ShowID:       show.ShowID,
			Name:         show.Name,
			Host:         show.Host,
			TopicFocus:   show.TopicFocus,
			StartsAround: future.Format("15:04"),
		})
		if len(upcoming) == 4 {
			break
		}
	}
	return upcoming
}

// ---------------------------------------------------------------------------
// GET /stats
// ---------------------------------------------------------------------------

type statsResponse struct {
	Uptime               string    `json:"uptime"`
	UptimeSecs           int64     `json:"uptime_seconds"`
	TracksPlayed         int       `json:"tracks_played"`
	TotalListenersServed int64     `json:"total_listeners_served"`
	CurrentListeners     int       `json:"current_listeners"`
	APIStarted           time.Time `json:"api_started"`
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	uptime := time.Since(s.startedAt)
	hours := int(uptime.Hours())
	minutes := int(uptime.Minutes()) % 60

	snapshot := s.stats.Snapshot()

	writeJSON(w, http.StatusOK, statsResponse{
		Uptime:               fmt.Sprintf("%dh %dm", hours, minutes),
		UptimeSecs:           int64(uptime.Seconds()),
		TracksPlayed:         snapshot.TracksPlayed,
		TotalListenersServed: snapshot.TotalListenersServed,
		CurrentListeners:     s.lc.get(),
		APIStarted:           s.startedAt,
	})
}

// ---------------------------------------------------------------------------
// GET /messages
// ---------------------------------------------------------------------------

func (s *Server) handleGetMessages(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.messages.Recent(20))
}

// ---------------------------------------------------------------------------
// POST /message
// ---------------------------------------------------------------------------

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)

	if wait, limited := s.messages.RateLimited(ip); limited {
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf("Please wait %ds before sending another message", wait))
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" || len(msg) > 280 {
		writeError(w, http.StatusBadRequest, "message must be 1–280 characters")
		return
	}

	s.messages.Add(msg, ip)
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

// clientIP extracts the client IP, preferring X-Forwarded-For.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (leftmost) address.
		if i := strings.IndexByte(xff, ','); i > 0 {
			xff = xff[:i]
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
