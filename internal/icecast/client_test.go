package icecast

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
