package api_test

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The unit suite in internal/metrics proves the registry counts and renders.
// What can only be proved against a real router is the part that comes from
// chi: that the label is the route PATTERN and never the path.

// scrape renders the live registry the test server records into.
func scrape(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a Docker daemon")
	}
	var b strings.Builder
	testAPI.Metrics().Render(&b)
	return b.String()
}

// counter reads one series' value, or 0 when the series does not exist yet.
// Counters are cumulative across the whole suite and other tests are serving
// requests into the same registry, so every assertion here is on a DELTA around
// a known call rather than on an absolute total.
func counter(t *testing.T, page, series string) int {
	t.Helper()
	for _, line := range strings.Split(page, "\n") {
		name, value, ok := strings.Cut(line, " ")
		if !ok || name != series {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			t.Fatalf("series %q has a non-integer value %q", series, value)
		}
		return n
	}
	return 0
}

// metricsSettleTimeout bounds how long a delta assertion waits for the
// registry to catch up. Generous, because it costs nothing when the
// observation is already there — awaitDelta returns on the first scrape that
// satisfies it, and only a genuine miss ever waits the full time.
const metricsSettleTimeout = 2 * time.Second

// awaitDelta polls until `series` has advanced by at least `want` from
// `before`, and returns the delta it settled on.
//
// Reading the counter ONCE after the request is a race, and it is the request
// that loses it. What the client synchronises on is the response; what this
// asserts on is the registry, and those are written at different moments by
// different goroutines. observe records in a deferred call that runs after the
// handler returns, while net/http can already have put the response on the
// wire — and httpexpect's Expect() resolves on the response HEADERS, because it
// wraps the body lazily (see bodyWrapper) and .Status() never reads it. So the
// test can be running again while the server goroutine has not yet reached its
// defer. On an idle box the gap is nanoseconds and never observed — 800
// attempts under full CPU contention did not reproduce it — but CI is a shared
// k8s runner, and one deschedule in the wrong microsecond is all it takes.
// TestMetricsRecordLatencyAlongsideTheCount failed there exactly once, on a
// commit whose diff was seven lines of a Playwright spec.
//
// Waiting is the honest fix rather than a bigger hammer: it removes the
// "asserted too early" failure and NOTHING else. Callers still compare the
// returned delta for equality, so an over-count fails as loudly as before, and
// an observation that never arrives still fails — just with a report of what
// the registry actually held, which is what this failure lacked the first time.
func awaitDelta(t *testing.T, series string, before, want int) int {
	t.Helper()
	deadline := time.Now().Add(metricsSettleTimeout)
	for {
		page := scrape(t)
		got := counter(t, page, series) - before
		if got >= want {
			return got
		}
		if time.Now().After(deadline) {
			// A series that never moved is usually a series recorded under a
			// DIFFERENT label — a route folded to "unmatched", a pattern that
			// grew a trailing slash. Print the neighbours so the next failure
			// names its own cause instead of being a number with no context.
			t.Logf("series %s reached %d of %d within %s; siblings on the page:\n%s",
				series, got, want, metricsSettleTimeout, siblingSeries(page, series))
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// siblingSeries returns every line sharing a metric name with series — the same
// metric under every label set the registry currently holds.
func siblingSeries(page, series string) string {
	name, _, ok := strings.Cut(series, "{")
	if !ok {
		return "  (no metric name to match)"
	}
	var b strings.Builder
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, name+"{") || strings.HasPrefix(line, name+" ") {
			b.WriteString("  " + line + "\n")
		}
	}
	if b.Len() == 0 {
		return "  (none — the metric is absent entirely)"
	}
	return strings.TrimRight(b.String(), "\n")
}

func TestMetricsLabelBySessionRoutePatternNotPath(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())

	const series = `iron_temple_http_requests_total{method="GET",route="/api/v1/sessions/{sessionId}",status="200"}`
	before := counter(t, scrape(t), series)

	e.GET("/sessions/{id}", sessionID).Expect().Status(http.StatusOK)
	e.GET("/sessions/{id}", sessionID).Expect().Status(http.StatusOK)

	if got := awaitDelta(t, series, before, 2); got != 2 {
		t.Errorf("pattern series moved by %d, want 2", got)
	}
}

// The failure this guards is the one that makes a metrics endpoint expensive:
// a label carrying the id means a new time series per session, so the scrape
// grows for the life of the training history.
func TestMetricsNeverLabelWithAnID(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())
	e.GET("/sessions/{id}", sessionID).Expect().Status(http.StatusOK)

	// Any route label with a bare number in a path segment is an id that
	// escaped the pattern.
	numericSegment := regexp.MustCompile(`route="[^"]*/\d+`)
	for _, line := range strings.Split(scrape(t), "\n") {
		if numericSegment.MatchString(line) {
			t.Errorf("a path id reached the route label:\n  %s", line)
		}
	}
}

// A 404's path is chosen by the caller. If it reached the label, anyone could
// add series to the registry by asking for paths that don't exist.
//
// Two layers do the collapsing, and both are asserted. A miss UNDER /api/v1 is
// caught by chi itself, which reports the subrouter's own wildcard pattern; a
// miss outside it matches nothing at all, has no pattern, and is folded by the
// registry. Either way the number of series a stranger can create is zero.
func TestMetricsFoldUnroutedRequestsIntoOneSeries(t *testing.T) {
	e := expectAnon(t)

	const wildcard = `iron_temple_http_requests_total{method="GET",route="/api/v1/*",status="404"}`
	const unmatched = `iron_temple_http_requests_total{method="GET",route="unmatched",status="404"}`
	before := scrape(t)
	beforeWildcard, beforeUnmatched := counter(t, before, wildcard), counter(t, before, unmatched)

	e.GET("/no-such-endpoint").Expect().Status(http.StatusNotFound)
	e.GET("/another/invented/path").Expect().Status(http.StatusNotFound)

	// Outside the API prefix entirely, where chi has no pattern to report.
	root := strings.TrimSuffix(baseURL, "/api/v1")
	for _, path := range []string{"/wp-login.php", "/.env"} {
		resp, err := http.Get(root + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
	}

	if got := awaitDelta(t, wildcard, beforeWildcard, 2); got != 2 {
		t.Errorf("the /api/v1 wildcard series moved by %d, want 2", got)
	}
	if got := awaitDelta(t, unmatched, beforeUnmatched, 2); got != 2 {
		t.Errorf("the unmatched series moved by %d, want 2", got)
	}

	// Scraped after both waits, not before: a label that arrives late is still
	// a leaked label, and a page read too early would miss it.
	page := scrape(t)
	for _, leaked := range []string{"no-such-endpoint", "invented", "wp-login", ".env"} {
		if strings.Contains(page, leaked) {
			t.Errorf("an unrouted path reached a label value: %q", leaked)
		}
	}
}

// A 401 is a served request and has to be counted as one — the rate of them is
// the signal that something is wrong with sessions.
func TestMetricsCountRejectedRequests(t *testing.T) {
	e := expectAnon(t)

	const series = `iron_temple_http_requests_total{method="GET",route="/api/v1/me",status="401"}`
	before := counter(t, scrape(t), series)

	e.GET("/me").Expect().Status(http.StatusUnauthorized)

	if got := awaitDelta(t, series, before, 1); got != 1 {
		t.Errorf("401 series moved by %d, want 1", got)
	}
}

// The pool is the resource this deployment is most likely to exhaust, so the
// gauge has to be live rather than a zero placeholder.
func TestMetricsReportThePoolItIsServingFrom(t *testing.T) {
	page := scrape(t)

	for _, want := range []string{
		`iron_temple_db_pool_connections{state="acquired"}`,
		`iron_temple_db_pool_connections{state="idle"}`,
		`iron_temple_db_pool_connections{state="total"}`,
		"iron_temple_db_pool_max_connections",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("scrape is missing %s", want)
		}
	}
	if counter(t, page, "iron_temple_db_pool_max_connections") <= 0 {
		t.Error("max connections reported as zero, so the source is not the real pool")
	}
}

// The histogram has to observe the same requests the counter does, or latency
// would be reported over a different population than the traffic.
func TestMetricsRecordLatencyAlongsideTheCount(t *testing.T) {
	e := expect(t)

	const series = `iron_temple_http_request_duration_seconds_count{method="GET",route="/api/v1/exercises"}`
	before := counter(t, scrape(t), series)

	e.GET("/exercises").Expect().Status(http.StatusOK)

	if got := awaitDelta(t, series, before, 1); got != 1 {
		t.Errorf("duration count moved by %d, want 1", got)
	}
}
