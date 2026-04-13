package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/writ-fm/go/internal/domain"
	"github.com/writ-fm/go/internal/nowplaying"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeState struct{ track nowplaying.Track }

func (f *fakeState) Get() nowplaying.Track { return f.track }

type fakeSchedule struct {
	show *domain.ResolvedShow
	err  error
}

func (f *fakeSchedule) Resolve(_ time.Time) (*domain.ResolvedShow, error) {
	return f.show, f.err
}

type fakeListeners struct{ n int; err error }

func (f *fakeListeners) Listeners(_ string) (int, error) { return f.n, f.err }

// newTestServer builds a Server wired with the given fakes and a temp messages file.
func newTestServer(t *testing.T, state TrackState, sched ScheduleResolver, lc ListenerCounter) *Server {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Addr:         ":0",
		Mount:        "/stream",
		MessagesFile: filepath.Join(dir, "messages.json"),
	}
	return New(cfg, state, sched, lc)
}

func defaultShow() *domain.ResolvedShow {
	return &domain.ResolvedShow{
		ShowID:       "midnight_signal",
		Name:         "Midnight Signal",
		Description:  "Late night transmissions",
		Host:         "signal_host",
		TopicFocus:   "underground culture",
		SegmentTypes: []string{"deep_dive", "music_essay"},
		BumperStyle:  "ambient",
	}
}

// ---------------------------------------------------------------------------
// GET /now-playing
// ---------------------------------------------------------------------------

func TestHandleNowPlaying_ReturnsTrackJSON(t *testing.T) {
	track := nowplaying.Track{
		ShowID:      "midnight_signal",
		ShowName:    "Midnight Signal",
		Type:        "talk",
		Name:        "Deep Dive",
		Host:        "signal_host",
		SegmentType: "deep_dive",
		UpdatedAt:   time.Now(),
	}
	srv := newTestServer(t, &fakeState{track}, &fakeSchedule{show: defaultShow()}, &fakeListeners{n: 3})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/now-playing", nil)
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["track"] != "Deep Dive" {
		t.Errorf("track = %v, want Deep Dive", got["track"])
	}
	if got["show"] != "Midnight Signal" {
		t.Errorf("show = %v, want Midnight Signal", got["show"])
	}
	// Listener count should be injected from icecast
	if got["listeners"].(float64) != 3 {
		t.Errorf("listeners = %v, want 3", got["listeners"])
	}
}

func TestHandleNowPlaying_RootAliasWorks(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want 200", w.Code)
	}
}

func TestHandleNowPlaying_HasNoCacheHeader(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	req := httptest.NewRequest(http.MethodGet, "/now-playing", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestHandleNowPlaying_TracksPlayed_Increments(t *testing.T) {
	srv := newTestServer(t, &fakeState{nowplaying.Track{Name: "Track A"}},
		&fakeSchedule{show: defaultShow()}, &fakeListeners{})

	for range 3 {
		srv.router.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/now-playing", nil))
	}
	srv.state = &fakeState{nowplaying.Track{Name: "Track B"}}
	srv.router.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/now-playing", nil))

	srv.mu.Lock()
	played := srv.tracksPlayed
	srv.mu.Unlock()

	if played != 2 {
		t.Errorf("tracksPlayed = %d, want 2", played)
	}
}

// ---------------------------------------------------------------------------
// GET /health
// ---------------------------------------------------------------------------

func TestHandleHealth_HealthyWhenIcecastUp(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{n: 1})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", got["status"])
	}
	components := got["components"].(map[string]any)
	icecast := components["icecast"].(map[string]any)
	if icecast["status"] != "up" {
		t.Errorf("icecast status = %v, want up", icecast["status"])
	}
}

func TestHandleHealth_DegradedWhenIcecastDown(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()},
		&fakeListeners{err: fmt.Errorf("connection refused")})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", got["status"])
	}
}

func TestHandleHealth_UptimeIncreases(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	srv.startedAt = time.Now().Add(-10 * time.Second)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	uptime := int64(got["uptime_seconds"].(float64))
	if uptime < 9 {
		t.Errorf("uptime_seconds = %d, want ≥ 9", uptime)
	}
}

// ---------------------------------------------------------------------------
// GET /schedule
// ---------------------------------------------------------------------------

func TestHandleSchedule_ReturnsCurrent(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	req := httptest.NewRequest(http.MethodGet, "/schedule", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	current := got["current"].(map[string]any)
	if current["show_id"] != "midnight_signal" {
		t.Errorf("show_id = %v, want midnight_signal", current["show_id"])
	}
	if current["name"] != "Midnight Signal" {
		t.Errorf("name = %v, want Midnight Signal", current["name"])
	}
}

func TestHandleSchedule_ErrorReturns500(t *testing.T) {
	srv := newTestServer(t, &fakeState{},
		&fakeSchedule{err: fmt.Errorf("no schedule block")}, &fakeListeners{})
	req := httptest.NewRequest(http.MethodGet, "/schedule", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /stats
// ---------------------------------------------------------------------------

func TestHandleStats_Fields(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{n: 5})
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	for _, key := range []string{"uptime", "uptime_seconds", "tracks_played", "current_listeners", "api_started"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in /stats response", key)
		}
	}
	if got["current_listeners"].(float64) != 5 {
		t.Errorf("current_listeners = %v, want 5", got["current_listeners"])
	}
}

// ---------------------------------------------------------------------------
// POST /message + GET /messages
// ---------------------------------------------------------------------------

func TestHandlePostMessage_Accepted(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	body := `{"message":"Hello from a listener!"}`
	req := httptest.NewRequest(http.MethodPost, "/message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["status"] != "received" {
		t.Errorf("status = %v, want received", got["status"])
	}
}

func TestHandlePostMessage_TooLong(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	msg := strings.Repeat("x", 281)
	body := fmt.Sprintf(`{"message":%q}`, msg)
	req := httptest.NewRequest(http.MethodPost, "/message", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandlePostMessage_RateLimited(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	body := `{"message":"first"}`
	makeReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/message", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "1.2.3.4:9999"
		return r
	}

	srv.router.ServeHTTP(httptest.NewRecorder(), makeReq())
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeReq())
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request status = %d, want 429", w.Code)
	}
}

func TestHandleGetMessages_ReturnsMessages(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})

	// Post a message first.
	body := `{"message":"test message"}`
	postReq := httptest.NewRequest(http.MethodPost, "/message", strings.NewReader(body))
	postReq.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(httptest.NewRecorder(), postReq)

	// Now GET /messages.
	req := httptest.NewRequest(http.MethodGet, "/messages", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var msgs []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &msgs)
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if msgs[0]["message"] != "test message" {
		t.Errorf("message = %v, want test message", msgs[0]["message"])
	}
	// IP must not be exposed.
	if _, ok := msgs[0]["ip"]; ok {
		t.Errorf("ip field should not be present in GET /messages response")
	}
}

// ---------------------------------------------------------------------------
// CORS
// ---------------------------------------------------------------------------

func TestCORSHeaders(t *testing.T) {
	srv := newTestServer(t, &fakeState{}, &fakeSchedule{show: defaultShow()}, &fakeListeners{})
	req := httptest.NewRequest(http.MethodGet, "/now-playing", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("CORS header missing or wrong: %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}
