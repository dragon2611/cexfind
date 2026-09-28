package cexfind

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchGateSpacingAndCancellation(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer server.Close()
	gate := newSearchGate(30 * time.Millisecond)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			req, _ := http.NewRequest("POST", server.URL, nil)
			res, err := gate.do(server.Client(), req)
			if err != nil {
				t.Error(err)
				return
			}
			res.Body.Close()
		})
	}
	wg.Wait()
	mu.Lock()
	for i := 1; i < len(times); i++ {
		if times[i].Sub(times[i-1]) < 25*time.Millisecond {
			t.Errorf("requests too close: %v", times)
		}
	}
	mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL, nil)
	if _, err := gate.do(server.Client(), req); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled request: %v", err)
	}
	gate.next = time.Now().Add(time.Hour)
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	req, _ = http.NewRequestWithContext(ctx, "POST", server.URL, nil)
	if _, err := gate.do(server.Client(), req); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("queued cancellation: %v", err)
	}
}

func TestSearchGate429StopsQueuedRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(429)
	}))
	defer server.Close()
	gate := newSearchGate(0)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			req, _ := http.NewRequest("POST", server.URL, nil)
			_, err := gate.do(server.Client(), req)
			var limited *RateLimitError
			if !errors.As(err, &limited) {
				t.Errorf("expected cooldown: %v", err)
			} else if time.Until(limited.Until) < 55*time.Second {
				t.Errorf("wrong cooldown: %v", limited)
			}
		})
	}
	wg.Wait()
	if requests.Load() != 1 {
		t.Fatalf("queued searches ignored 429: %d", requests.Load())
	}
	gate.blockedUntil = time.Now().Add(-time.Second)
	req, _ := http.NewRequest("POST", server.URL, nil)
	gate.do(server.Client(), req)
	if requests.Load() != 2 {
		t.Fatal("expired cooldown prevented retry")
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		value string
		want  time.Time
	}{
		{"20", now.Add(20 * time.Second)},
		{now.Add(2 * time.Minute).Format(http.TimeFormat), now.Add(2 * time.Minute)},
		{"", now.Add(time.Minute)},
		{"invalid", now.Add(time.Minute)},
		{"-1", now.Add(time.Minute)},
		{now.Add(-time.Minute).Format(http.TimeFormat), now.Add(time.Minute)},
		{"0", now.Add(time.Second)},
	} {
		if got := retryAfter(tc.value, now); !got.Equal(tc.want) {
			t.Errorf("%q: got %v want %v", tc.value, got, tc.want)
		}
	}
}

func TestPostQueryReportsHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "blocked", 403) }))
	defer server.Close()
	oldURL := URL
	URL = server.URL
	defer func() { URL = oldURL }()
	if _, err := postQuery(server.Client(), []byte(`{}`)); err == nil || err.Error() != "search backend returned HTTP 403" {
		t.Fatalf("unexpected status error: %v", err)
	}
}
