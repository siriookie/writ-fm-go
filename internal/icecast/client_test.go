package icecast

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func makeServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

const singleSourceJSON = `{
  "icestats": {
    "source": {
      "listenurl": "http://localhost:8000/stream",
      "listeners": 3
    }
  }
}`

const arraySourceJSON = `{
  "icestats": {
    "source": [
      {"listenurl": "http://localhost:8000/stream", "listeners": 3},
      {"listenurl": "http://localhost:8000/music",  "listeners": 1}
    ]
  }
}`

const noSourceJSON = `{"icestats": {}}`

func TestClient_Listeners_SingleSource(t *testing.T) {
	srv := makeServer(t, singleSourceJSON, 200)
	defer srv.Close()
	n, err := NewClient(srv.URL).Listeners("/stream")
	if err != nil {
		t.Fatalf("Listeners: %v", err)
	}
	if n != 3 {
		t.Errorf("Listeners = %d, want 3", n)
	}
}

func TestClient_Listeners_ArraySources_FirstMount(t *testing.T) {
	srv := makeServer(t, arraySourceJSON, 200)
	defer srv.Close()
	n, err := NewClient(srv.URL).Listeners("/stream")
	if err != nil {
		t.Fatalf("Listeners: %v", err)
	}
	if n != 3 {
		t.Errorf("Listeners = %d, want 3", n)
	}
}

func TestClient_Listeners_ArraySources_SecondMount(t *testing.T) {
	srv := makeServer(t, arraySourceJSON, 200)
	defer srv.Close()
	n, err := NewClient(srv.URL).Listeners("/music")
	if err != nil {
		t.Fatalf("Listeners: %v", err)
	}
	if n != 1 {
		t.Errorf("Listeners = %d, want 1", n)
	}
}

func TestClient_Listeners_NoSources(t *testing.T) {
	srv := makeServer(t, noSourceJSON, 200)
	defer srv.Close()
	n, err := NewClient(srv.URL).Listeners("/stream")
	if err != nil {
		t.Fatalf("Listeners: %v", err)
	}
	if n != 0 {
		t.Errorf("Listeners = %d, want 0", n)
	}
}

func TestClient_Listeners_MountpointNotFound(t *testing.T) {
	srv := makeServer(t, singleSourceJSON, 200)
	defer srv.Close()
	n, err := NewClient(srv.URL).Listeners("/other")
	if err != nil {
		t.Fatalf("Listeners: %v", err)
	}
	if n != 0 {
		t.Errorf("Listeners = %d, want 0", n)
	}
}

func TestClient_Listeners_MountpointWithoutLeadingSlash(t *testing.T) {
	srv := makeServer(t, singleSourceJSON, 200)
	defer srv.Close()
	// "stream" (no "/") should match "/stream"
	n, err := NewClient(srv.URL).Listeners("stream")
	if err != nil {
		t.Fatalf("Listeners: %v", err)
	}
	if n != 3 {
		t.Errorf("Listeners = %d, want 3", n)
	}
}

func TestClient_Listeners_ServerError(t *testing.T) {
	srv := makeServer(t, "", 500)
	defer srv.Close()
	_, err := NewClient(srv.URL).Listeners("/stream")
	if err == nil {
		t.Fatal("expected error for HTTP 500 response")
	}
}

func TestClient_Listeners_MalformedJSON(t *testing.T) {
	srv := makeServer(t, "not json at all", 200)
	defer srv.Close()
	_, err := NewClient(srv.URL).Listeners("/stream")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestClient_Listeners_TrailingSlashInBaseURL(t *testing.T) {
	srv := makeServer(t, singleSourceJSON, 200)
	defer srv.Close()
	// NewClient strips trailing slash so status URL is still correct
	n, err := NewClient(srv.URL + "/").Listeners("/stream")
	if err != nil {
		t.Fatalf("Listeners: %v", err)
	}
	if n != 3 {
		t.Errorf("Listeners = %d, want 3", n)
	}
}

// ---- cache tests -------------------------------------------------------

func newCountingServer(t *testing.T, bodies []string, statuses []int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := calls
		calls++
		if i >= len(bodies) {
			i = len(bodies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statuses[i])
		_, _ = w.Write([]byte(bodies[i]))
	}))
	return srv, &calls
}

func TestClient_Listeners_CacheHit(t *testing.T) {
	srv, calls := newCountingServer(t,
		[]string{singleSourceJSON, singleSourceJSON},
		[]int{200, 200},
	)
	defer srv.Close()
	c := newClientWithTTL(srv.URL, 15*time.Second)

	if _, err := c.Listeners("/stream"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.Listeners("/stream"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if *calls != 1 {
		t.Errorf("expected 1 HTTP call (cache hit), got %d", *calls)
	}
}

func TestClient_Listeners_CacheMiss_AfterExpiry(t *testing.T) {
	srv, calls := newCountingServer(t,
		[]string{singleSourceJSON, singleSourceJSON},
		[]int{200, 200},
	)
	defer srv.Close()
	c := newClientWithTTL(srv.URL, time.Millisecond)

	if _, err := c.Listeners("/stream"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	time.Sleep(5 * time.Millisecond) // let TTL expire
	if _, err := c.Listeners("/stream"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if *calls != 2 {
		t.Errorf("expected 2 HTTP calls after TTL expiry, got %d", *calls)
	}
}

func TestClient_Listeners_CacheFallback_OnError(t *testing.T) {
	// First request succeeds; second (after TTL) fails → fallback to cached value, no error.
	srv, _ := newCountingServer(t,
		[]string{singleSourceJSON, ""},
		[]int{200, 500},
	)
	defer srv.Close()
	c := newClientWithTTL(srv.URL, time.Millisecond)

	n, err := c.Listeners("/stream")
	if err != nil || n != 3 {
		t.Fatalf("first call: n=%d err=%v, want n=3 err=nil", n, err)
	}

	time.Sleep(5 * time.Millisecond)
	n, err = c.Listeners("/stream")
	if err != nil {
		t.Errorf("expected no error on fallback, got %v", err)
	}
	if n != 3 {
		t.Errorf("expected cached n=3, got %d", n)
	}
}

func TestClient_Listeners_DifferentMountpoints_IndependentCache(t *testing.T) {
	srv, calls := newCountingServer(t,
		[]string{arraySourceJSON, arraySourceJSON},
		[]int{200, 200},
	)
	defer srv.Close()
	c := newClientWithTTL(srv.URL, 15*time.Second)

	if _, err := c.Listeners("/stream"); err != nil {
		t.Fatalf("/stream: %v", err)
	}
	if _, err := c.Listeners("/music"); err != nil {
		t.Fatalf("/music: %v", err)
	}

	// Both mountpoints share one HTTP response; Icecast /status-json.xsl returns all sources.
	// Second call should also be a cache hit (same URL fetched).
	if *calls != 1 {
		t.Errorf("expected 1 HTTP call (both mounts from same response), got %d", *calls)
	}
}
