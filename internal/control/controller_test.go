package control

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestController_NextSegmentReturnsChannel(t *testing.T) {
	c := NewController()
	ch := c.NextSegment()
	if ch == nil {
		t.Fatal("NextSegment returned nil channel")
	}
}

func TestController_SkipClosesChannel(t *testing.T) {
	c := NewController()
	ch := c.NextSegment()
	c.Skip()
	select {
	case <-ch:
		// good — channel was closed by Skip
	default:
		t.Fatal("Skip() did not close the skip channel")
	}
}

func TestController_SkipBeforeNextSegmentIsNoop(t *testing.T) {
	c := NewController()
	c.Skip() // no active segment — must not panic
}

func TestController_DoubleSkipIsNoop(t *testing.T) {
	c := NewController()
	_ = c.NextSegment()
	c.Skip()
	c.Skip() // second skip on closed channel — must not panic
}

func TestController_NextSegmentAfterSkipIsUnclosed(t *testing.T) {
	c := NewController()
	_ = c.NextSegment()
	c.Skip()
	next := c.NextSegment()
	select {
	case <-next:
		t.Fatal("channel from NextSegment after Skip should not be closed")
	default:
		// good — fresh channel
	}
}

func TestController_ServeHTTP_PostSkipReturns204(t *testing.T) {
	c := NewController()
	_ = c.NextSegment()
	req := httptest.NewRequest(http.MethodPost, "/skip", nil)
	rr := httptest.NewRecorder()
	c.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("POST /skip: status = %d, want 204", rr.Code)
	}
}

func TestController_ServeHTTP_PostSkipSignalsChannel(t *testing.T) {
	c := NewController()
	ch := c.NextSegment()
	req := httptest.NewRequest(http.MethodPost, "/skip", nil)
	rr := httptest.NewRecorder()
	c.ServeHTTP(rr, req)
	select {
	case <-ch:
		// good — HTTP skip propagated to the channel
	default:
		t.Fatal("POST /skip did not close the skip channel")
	}
}

func TestController_ServeHTTP_GetSkipReturns405(t *testing.T) {
	c := NewController()
	req := httptest.NewRequest(http.MethodGet, "/skip", nil)
	rr := httptest.NewRecorder()
	c.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /skip: status = %d, want 405", rr.Code)
	}
}

func TestController_ServeHTTP_UnknownPathReturns404(t *testing.T) {
	c := NewController()
	req := httptest.NewRequest(http.MethodPost, "/other", nil)
	rr := httptest.NewRecorder()
	c.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("POST /other: status = %d, want 404", rr.Code)
	}
}
