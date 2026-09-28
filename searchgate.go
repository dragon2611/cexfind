package cexfind

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const searchRequestInterval = 500 * time.Millisecond

// Shared by all finders in this process, including clients with different proxies.
// Separate CLI processes do not share this limiter or their search caches.
var sharedSearchGate = newSearchGate(searchRequestInterval)

// RateLimitError reports when the backend will accept another search attempt.
// The client never automatically retries a rate-limited request.
type RateLimitError struct{ Until time.Time }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("search backend rate limited requests; try again after %s", e.Until.Format(time.RFC3339))
}

type searchGate struct {
	token        chan struct{}
	interval     time.Duration
	next         time.Time
	blockedUntil time.Time
}

func newSearchGate(interval time.Duration) *searchGate {
	g := &searchGate{token: make(chan struct{}, 1), interval: interval}
	g.token <- struct{}{}
	return g
}

// Hold the token until response headers arrive so a 429 can stop queued searches.
func (g *searchGate) do(client *http.Client, request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.token:
	}
	defer func() { g.token <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if time.Now().Before(g.blockedUntil) {
		return nil, &RateLimitError{Until: g.blockedUntil}
	}
	if err := waitForSearch(ctx, time.Until(g.next)); err != nil {
		return nil, err
	}
	g.next = time.Now().Add(g.interval)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusTooManyRequests {
		g.blockedUntil = retryAfter(response.Header.Get("Retry-After"), time.Now())
		response.Body.Close()
		return nil, &RateLimitError{Until: g.blockedUntil}
	}
	return response, nil
}

func waitForSearch(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func retryAfter(value string, now time.Time) time.Time {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		// Avoid overflowing time.Duration for unusually large server values.
		maxSeconds := int64((1<<63 - 1) / int64(time.Second))
		seconds = min(max(seconds, 1), maxSeconds)
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if until, err := http.ParseTime(value); err == nil && until.After(now) {
		return until
	}
	return now.Add(time.Minute)
}
