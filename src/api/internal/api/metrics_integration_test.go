package api_test

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
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

func TestMetricsLabelBySessionRoutePatternNotPath(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())

	const series = `iron_temple_http_requests_total{method="GET",route="/api/v1/sessions/{sessionId}",status="200"}`
	before := counter(t, scrape(t), series)

	e.GET("/sessions/{id}", sessionID).Expect().Status(http.StatusOK)
	e.GET("/sessions/{id}", sessionID).Expect().Status(http.StatusOK)

	if got := counter(t, scrape(t), series) - before; got != 2 {
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

	page := scrape(t)
	if got := counter(t, page, wildcard) - beforeWildcard; got != 2 {
		t.Errorf("the /api/v1 wildcard series moved by %d, want 2", got)
	}
	if got := counter(t, page, unmatched) - beforeUnmatched; got != 2 {
		t.Errorf("the unmatched series moved by %d, want 2", got)
	}
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

	if got := counter(t, scrape(t), series) - before; got != 1 {
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

	if got := counter(t, scrape(t), series) - before; got != 1 {
		t.Errorf("duration count moved by %d, want 1", got)
	}
}
