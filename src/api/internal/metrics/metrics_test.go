package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// render returns the exposition page as a string, which every assertion below
// reads. The page is the package's whole contract — a counter that increments
// but renders wrong is a counter that does not work.
func render(t *testing.T, r *Registry) string {
	t.Helper()
	var b strings.Builder
	r.Render(&b)
	return b.String()
}

func mustContain(t *testing.T, page, line string) {
	t.Helper()
	if !strings.Contains(page, line) {
		t.Errorf("page is missing:\n  %s\ngot:\n%s", line, page)
	}
}

func TestRequestCounterSeriesPerMethodRouteStatus(t *testing.T) {
	r := New("v1.2.3", "production")

	r.ObserveRequest(http.MethodGet, "/api/v1/sessions", 200, 12*time.Millisecond)
	r.ObserveRequest(http.MethodGet, "/api/v1/sessions", 200, 30*time.Millisecond)
	r.ObserveRequest(http.MethodGet, "/api/v1/sessions", 500, time.Second)
	r.ObserveRequest(http.MethodPost, "/api/v1/sessions", 201, 40*time.Millisecond)

	page := render(t, r)
	mustContain(t, page, `iron_temple_http_requests_total{method="GET",route="/api/v1/sessions",status="200"} 2`)
	mustContain(t, page, `iron_temple_http_requests_total{method="GET",route="/api/v1/sessions",status="500"} 1`)
	mustContain(t, page, `iron_temple_http_requests_total{method="POST",route="/api/v1/sessions",status="201"} 1`)
}

// The route label has to be the pattern, not the path — but the middleware can
// only supply a pattern when one matched. A 404's path is chosen by whoever
// sent it, so every miss has to collapse onto one series.
func TestUnmatchedRouteCollapsesToOneSeries(t *testing.T) {
	r := New("dev", "development")

	r.ObserveRequest(http.MethodGet, "", 404, time.Millisecond)
	r.ObserveRequest(http.MethodGet, "", 404, time.Millisecond)

	page := render(t, r)
	mustContain(t, page, `iron_temple_http_requests_total{method="GET",route="unmatched",status="404"} 2`)
}

// HTTP allows any token as a method, so the label would otherwise be an
// unbounded dimension anyone could write to.
func TestUnknownMethodsFoldIntoOther(t *testing.T) {
	r := New("dev", "development")

	r.ObserveRequest("PROPFIND", "/api/v1/health", 405, time.Millisecond)
	r.ObserveRequest("BREW", "/api/v1/health", 405, time.Millisecond)

	page := render(t, r)
	mustContain(t, page, `iron_temple_http_requests_total{method="other",route="/api/v1/health",status="405"} 2`)
	if strings.Contains(page, "PROPFIND") || strings.Contains(page, "BREW") {
		t.Error("an invented method reached a label value")
	}
}

func TestHistogramBucketsAreCumulative(t *testing.T) {
	r := New("dev", "development")

	// 12ms lands in the 0.025 bucket and every wider one; 300ms in 0.5 upwards.
	r.ObserveRequest(http.MethodGet, "/api/v1/racked", 200, 12*time.Millisecond)
	r.ObserveRequest(http.MethodGet, "/api/v1/racked", 200, 300*time.Millisecond)

	page := render(t, r)
	labels := `method="GET",route="/api/v1/racked"`
	mustContain(t, page, `iron_temple_http_request_duration_seconds_bucket{`+labels+`,le="0.01"} 0`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_bucket{`+labels+`,le="0.025"} 1`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_bucket{`+labels+`,le="0.25"} 1`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_bucket{`+labels+`,le="0.5"} 2`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_bucket{`+labels+`,le="+Inf"} 2`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_count{`+labels+`} 2`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_sum{`+labels+`} 0.312`)
}

// A request slower than the widest bucket must still be counted in +Inf and in
// the sum, or a stalled endpoint would look like no traffic at all.
func TestObservationBeyondLastBucketStillCounts(t *testing.T) {
	r := New("dev", "development")

	r.ObserveRequest(http.MethodGet, "/api/v1/racked", 200, 30*time.Second)

	page := render(t, r)
	labels := `method="GET",route="/api/v1/racked"`
	mustContain(t, page, `iron_temple_http_request_duration_seconds_bucket{`+labels+`,le="10"} 0`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_bucket{`+labels+`,le="+Inf"} 1`)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_sum{`+labels+`} 30`)
}

// Latency is not split by status: a route's histogram answers "how long does
// this take", and dividing it by outcome halves the sample count for nothing.
func TestDurationIsNotKeyedByStatus(t *testing.T) {
	r := New("dev", "development")

	r.ObserveRequest(http.MethodGet, "/api/v1/me", 200, 10*time.Millisecond)
	r.ObserveRequest(http.MethodGet, "/api/v1/me", 401, 10*time.Millisecond)

	page := render(t, r)
	mustContain(t, page, `iron_temple_http_request_duration_seconds_count{method="GET",route="/api/v1/me"} 2`)
}

func TestInFlightRisesAndFalls(t *testing.T) {
	r := New("dev", "development")

	first := r.RequestStarted()
	second := r.RequestStarted()
	mustContain(t, render(t, r), "iron_temple_http_requests_in_flight 2")

	first(http.MethodGet, "/api/v1/me", 200, time.Millisecond)
	mustContain(t, render(t, r), "iron_temple_http_requests_in_flight 1")

	second(http.MethodGet, "/api/v1/me", 200, time.Millisecond)
	page := render(t, r)
	mustContain(t, page, "iron_temple_http_requests_in_flight 0")
	// The completion callback records the request as well as releasing the gauge.
	mustContain(t, page, `iron_temple_http_requests_total{method="GET",route="/api/v1/me",status="200"} 2`)
}

// No pool registered and an empty pool are different claims, so the family is
// absent rather than zero until a source exists.
func TestPoolMetricsAbsentUntilSourceRegistered(t *testing.T) {
	r := New("dev", "development")

	if strings.Contains(render(t, r), "db_pool_connections") {
		t.Fatal("pool metrics rendered without a source")
	}

	r.SetPoolSource(func() PoolStats {
		return PoolStats{Acquired: 2, Idle: 3, Total: 5, Max: 10}
	})

	page := render(t, r)
	mustContain(t, page, `iron_temple_db_pool_connections{state="acquired"} 2`)
	mustContain(t, page, `iron_temple_db_pool_connections{state="idle"} 3`)
	mustContain(t, page, `iron_temple_db_pool_connections{state="total"} 5`)
	mustContain(t, page, "iron_temple_db_pool_max_connections 10")
}

// The source is read while the scrape is rendered, so the numbers are the ones
// true when Prometheus asked rather than whenever a ticker last fired.
func TestPoolSourceIsReadAtScrapeTime(t *testing.T) {
	r := New("dev", "development")
	acquired := int32(1)
	r.SetPoolSource(func() PoolStats { return PoolStats{Acquired: acquired} })

	mustContain(t, render(t, r), `iron_temple_db_pool_connections{state="acquired"} 1`)
	acquired = 7
	mustContain(t, render(t, r), `iron_temple_db_pool_connections{state="acquired"} 7`)
}

func TestBuildInfoCarriesVersionAndEnvironment(t *testing.T) {
	r := New("v2.0.0", "production")

	mustContain(t, render(t, r),
		`iron_temple_build_info{version="v2.0.0",environment="production"} 1`)
}

// A version string is stamped in at build time. One stray quote in it would
// otherwise produce a page Prometheus rejects in full — every metric lost, not
// just the label.
func TestLabelValuesAreEscaped(t *testing.T) {
	r := New(`v1"0`+"\n"+`\x`, "production")

	mustContain(t, render(t, r), `iron_temple_build_info{version="v1\"0\n\\x",environment="production"} 1`)
}

func TestEveryFamilyDeclaresHelpAndType(t *testing.T) {
	r := New("dev", "development")
	r.SetPoolSource(func() PoolStats { return PoolStats{} })
	r.ObserveRequest(http.MethodGet, "/api/v1/me", 200, time.Millisecond)

	page := render(t, r)
	for _, line := range strings.Split(strings.TrimSpace(page), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, _, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("sample line has no value: %q", line)
		}
		// Strip labels and any histogram suffix to reach the family name.
		if i := strings.Index(name, "{"); i >= 0 {
			name = name[:i]
		}
		for _, suffix := range []string{"_bucket", "_sum", "_count"} {
			if strings.HasSuffix(name, suffix) && strings.Contains(name, "duration_seconds") {
				name = strings.TrimSuffix(name, suffix)
			}
		}
		if !strings.Contains(page, "# TYPE "+name+" ") {
			t.Errorf("sample %q has no TYPE declaration", name)
		}
		if !strings.Contains(page, "# HELP "+name+" ") {
			t.Errorf("sample %q has no HELP declaration", name)
		}
	}
}

// Sorted output is what makes the page diffable and these tests comparable.
func TestSeriesAreRenderedInStableOrder(t *testing.T) {
	r := New("dev", "development")
	for _, route := range []string{"/api/v1/sessions", "/api/v1/me", "/api/v1/exercises"} {
		r.ObserveRequest(http.MethodGet, route, 200, time.Millisecond)
	}

	// Compared over the counter family alone. The runtime gauges (heap, GC,
	// goroutines) are read fresh on every scrape and are supposed to move, so
	// asserting over the whole page would be asserting the process is idle.
	counters := func() string {
		var kept []string
		for _, line := range strings.Split(render(t, r), "\n") {
			if strings.HasPrefix(line, namespace+"http_requests_total{") {
				kept = append(kept, line)
			}
		}
		return strings.Join(kept, "\n")
	}

	first := counters()
	for range 5 {
		if page := counters(); page != first {
			t.Fatalf("two renders of an unchanged registry disagreed:\n%s\n---\n%s", first, page)
		}
	}

	me := strings.Index(first, `route="/api/v1/me",status="200"`)
	sessions := strings.Index(first, `route="/api/v1/sessions",status="200"`)
	exercises := strings.Index(first, `route="/api/v1/exercises",status="200"`)
	if exercises >= me || me >= sessions {
		t.Errorf("series are not sorted by route: exercises=%d me=%d sessions=%d", exercises, me, sessions)
	}
}

func TestHandlerServesTheExpositionContentType(t *testing.T) {
	r := New("dev", "development")
	r.ObserveRequest(http.MethodGet, "/api/v1/me", 200, time.Millisecond)

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", got)
	}
	mustContain(t, rec.Body.String(), "iron_temple_http_requests_total")
}

func TestHandlerRefusesWrites(t *testing.T) {
	r := New("dev", "development")

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/metrics", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Errorf("Allow = %q, want %q", got, "GET, HEAD")
	}
}

// Requests are served concurrently and a scrape lands in the middle of them.
// Run with -race, this is the assertion that the whole registry is guarded.
func TestConcurrentObservationAndScrape(t *testing.T) {
	r := New("dev", "development")
	r.SetPoolSource(func() PoolStats { return PoolStats{Idle: 1} })

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				done := r.RequestStarted()
				done(http.MethodGet, "/api/v1/sessions", 200, time.Duration(i)*time.Millisecond)
			}
		}()
	}
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				var sink strings.Builder
				r.Render(&sink)
			}
		}()
	}
	wg.Wait()

	page := render(t, r)
	mustContain(t, page, `iron_temple_http_requests_total{method="GET",route="/api/v1/sessions",status="200"} 800`)
	mustContain(t, page, "iron_temple_http_requests_in_flight 0")
}
