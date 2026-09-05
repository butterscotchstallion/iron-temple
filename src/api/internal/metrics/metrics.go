// Package metrics exposes the API's own numbers in Prometheus text format.
//
// Hand-rolled, and deliberately so. The module is vendored and
// prometheus/client_golang is not in the tree; pulling it in to count requests
// would add a dependency graph an order of magnitude larger than the thing it
// measures. The exposition format is a documented text protocol, the metric set
// here is fixed and small, and the whole encoder is one function.
//
// Everything is stdlib. In particular this package does NOT import chi or pgx:
// the route pattern arrives as a string from a middleware that already has chi
// (internal/api), and pool statistics arrive through a callback the server
// registers. That keeps the numbers separable from what produces them, and
// keeps this package testable without a router or a database.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// namespace prefixes every metric. One prefix for the app's own numbers and the
// runtime's alike, rather than the bare `go_*` names client_golang would emit —
// nothing else is exported from this process, and an unambiguous prefix means
// adopting a real client library later cannot collide with what is already
// scraped.
const namespace = "iron_temple_"

// buckets are the histogram's upper bounds in seconds, cumulative.
//
// Sized for a DB-backed JSON API on a homelab cluster: the interesting
// resolution is single-digit milliseconds to a couple of hundred, and anything
// past a second is already a problem whose exact shape doesn't matter. The last
// finite bucket is deliberately well above the client's patience so the +Inf
// bucket means "effectively never returned", not "slower than usual".
var buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// knownMethods bounds the `method` label.
//
// Cardinality protection, not tidiness. A route pattern comes from the router
// and is therefore drawn from a fixed set, but the method comes off the request
// line and HTTP allows any token there — so an unauthenticated scanner spraying
// invented verbs at the API would otherwise mint a new time series per verb and
// grow this map without limit. Anything unrecognised is folded into "other".
var knownMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true,
	http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	http.MethodOptions: true,
}

// UnmatchedRoute is the `route` label for a request that matched no pattern.
//
// The raw path cannot be used for these: a 404 is exactly the request whose
// path is attacker-chosen, and labelling by it would let anyone add series at
// will. Every miss shares one series, which is all the signal a 404 rate needs.
const UnmatchedRoute = "unmatched"

// PoolStats is the slice of a pgxpool.Stat this package reports. Declared here
// rather than taking the pgx type so the package stays dependency-free; the
// server adapts one to the other where it registers the source.
type PoolStats struct {
	Acquired int32
	Idle     int32
	Total    int32
	Max      int32
}

// requestKey identifies one series of the request counter.
type requestKey struct {
	method string
	route  string
	status int
}

// routeKey identifies one series of the duration histogram. Deliberately not
// keyed by status: the question a latency histogram answers is "how long does
// this route take", and splitting it by outcome divides the sample count
// without changing the answer.
type routeKey struct {
	method string
	route  string
}

// histogram is a cumulative bucket set plus the sum and count Prometheus needs
// to derive an average.
type histogram struct {
	counts []uint64 // per-bucket, same order and length as buckets
	sum    float64
	count  uint64
}

func (h *histogram) observe(seconds float64) {
	h.sum += seconds
	h.count++
	for i, upper := range buckets {
		if seconds <= upper {
			h.counts[i]++
		}
	}
}

// Registry holds every metric this process reports.
//
// One mutex over the whole thing rather than an atomic per series: a scrape has
// to walk all of it anyway, and the write side is one map lookup per request
// against a handful of contended cache lines. At this app's traffic — one
// lifter, a few requests per set — a lock-free design would be measuring
// nothing more accurately.
type Registry struct {
	mu       sync.Mutex
	requests map[requestKey]uint64
	duration map[routeKey]*histogram
	inFlight int64

	// poolSource is read at scrape time rather than polled on a ticker, so the
	// numbers are the ones true when Prometheus asked. Nil until the server
	// registers one, which is how tests and the unit suite avoid needing a
	// database to render a page of metrics.
	poolSource func() PoolStats

	version     string
	environment string
	startedAt   time.Time
}

// New builds a Registry. version and environment are reported as build_info
// labels, the same pair the health endpoint serves, so a scrape can tell which
// release produced a series without joining against anything else.
func New(version, environment string) *Registry {
	return &Registry{
		requests:    make(map[requestKey]uint64),
		duration:    make(map[routeKey]*histogram),
		version:     version,
		environment: environment,
		startedAt:   time.Now(),
	}
}

// SetPoolSource registers where connection-pool numbers come from. Safe to
// leave unset; the pool metrics are then simply absent from the output rather
// than reported as zero, because zero idle connections and no pool at all are
// very different claims.
func (r *Registry) SetPoolSource(fn func() PoolStats) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.poolSource = fn
}

// RequestStarted marks a request as in flight. The returned function marks it
// done and records the outcome, so a caller cannot increment the gauge without
// also arranging to decrement it:
//
//	done := reg.RequestStarted()
//	defer func() { done(method, route, status, time.Since(start)) }()
func (r *Registry) RequestStarted() func(method, route string, status int, d time.Duration) {
	r.mu.Lock()
	r.inFlight++
	r.mu.Unlock()

	return func(method, route string, status int, d time.Duration) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.inFlight--
		r.observeLocked(method, route, status, d)
	}
}

// ObserveRequest records a completed request without the in-flight accounting.
// For callers that only want the counters — and for tests.
func (r *Registry) ObserveRequest(method, route string, status int, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observeLocked(method, route, status, d)
}

func (r *Registry) observeLocked(method, route string, status int, d time.Duration) {
	method = normaliseMethod(method)
	if route == "" {
		route = UnmatchedRoute
	}

	r.requests[requestKey{method: method, route: route, status: status}]++

	key := routeKey{method: method, route: route}
	h := r.duration[key]
	if h == nil {
		h = &histogram{counts: make([]uint64, len(buckets))}
		r.duration[key] = h
	}
	h.observe(d.Seconds())
}

// normaliseMethod folds an unrecognised verb into a single label. See
// knownMethods for why that matters.
func normaliseMethod(method string) string {
	if knownMethods[method] {
		return method
	}
	return "other"
}

// Handler serves the exposition page. Mount it on an address that is not the
// public one — see cmd/server for where that happens and why.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if req.Method == http.MethodHead {
			return
		}
		r.Render(w)
	})
}

// Render writes the exposition page.
//
// Named Render rather than WriteTo because the latter is a signature Go
// reserves by convention (io.WriterTo returns a count and an error), and vet
// treats a method that borrows the name without the shape as a mistake.
//
// Every family is emitted whole — HELP, TYPE, then its samples — and samples
// within a family are sorted. Prometheus itself does not care about the order,
// but a diffable page is worth a sort of a few dozen strings: it makes the
// output testable by comparison and readable by a human with curl.
func (r *Registry) Render(w io.Writer) {
	r.mu.Lock()
	requests := make(map[requestKey]uint64, len(r.requests))
	for k, v := range r.requests {
		requests[k] = v
	}
	duration := make(map[routeKey]histogram, len(r.duration))
	for k, v := range r.duration {
		counts := make([]uint64, len(v.counts))
		copy(counts, v.counts)
		duration[k] = histogram{counts: counts, sum: v.sum, count: v.count}
	}
	inFlight := r.inFlight
	poolSource := r.poolSource
	version, environment, startedAt := r.version, r.environment, r.startedAt
	r.mu.Unlock()

	var b strings.Builder

	writeFamily(&b, "build_info", "Version and environment of the running binary, as a constant 1.", "gauge")
	fmt.Fprintf(&b, "%sbuild_info{version=%s,environment=%s} 1\n",
		namespace, quote(version), quote(environment))

	writeFamily(&b, "process_start_time_seconds", "Unix time at which this process started.", "gauge")
	fmt.Fprintf(&b, "%sprocess_start_time_seconds %s\n", namespace, formatFloat(float64(startedAt.UnixNano())/1e9))

	writeFamily(&b, "http_requests_total", "Requests served, by method, route pattern and status.", "counter")
	for _, key := range sortedRequestKeys(requests) {
		fmt.Fprintf(&b, "%shttp_requests_total{method=%s,route=%s,status=%s} %d\n",
			namespace, quote(key.method), quote(key.route), quote(strconv.Itoa(key.status)), requests[key])
	}

	writeFamily(&b, "http_requests_in_flight", "Requests currently being served.", "gauge")
	fmt.Fprintf(&b, "%shttp_requests_in_flight %d\n", namespace, inFlight)

	writeFamily(&b, "http_request_duration_seconds", "Request latency, by method and route pattern.", "histogram")
	for _, key := range sortedRouteKeys(duration) {
		h := duration[key]
		labels := fmt.Sprintf("method=%s,route=%s", quote(key.method), quote(key.route))
		for i, upper := range buckets {
			fmt.Fprintf(&b, "%shttp_request_duration_seconds_bucket{%s,le=%s} %d\n",
				namespace, labels, quote(formatFloat(upper)), h.counts[i])
		}
		fmt.Fprintf(&b, "%shttp_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", namespace, labels, h.count)
		fmt.Fprintf(&b, "%shttp_request_duration_seconds_sum{%s} %s\n", namespace, labels, formatFloat(h.sum))
		fmt.Fprintf(&b, "%shttp_request_duration_seconds_count{%s} %d\n", namespace, labels, h.count)
	}

	if poolSource != nil {
		stats := poolSource()
		writeFamily(&b, "db_pool_connections", "Connections in the pgx pool, by state.", "gauge")
		fmt.Fprintf(&b, "%sdb_pool_connections{state=\"acquired\"} %d\n", namespace, stats.Acquired)
		fmt.Fprintf(&b, "%sdb_pool_connections{state=\"idle\"} %d\n", namespace, stats.Idle)
		fmt.Fprintf(&b, "%sdb_pool_connections{state=\"total\"} %d\n", namespace, stats.Total)

		writeFamily(&b, "db_pool_max_connections", "Upper bound on connections the pool will open.", "gauge")
		fmt.Fprintf(&b, "%sdb_pool_max_connections %d\n", namespace, stats.Max)
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	writeFamily(&b, "go_goroutines", "Goroutines currently running.", "gauge")
	fmt.Fprintf(&b, "%sgo_goroutines %d\n", namespace, runtime.NumGoroutine())

	writeFamily(&b, "go_heap_alloc_bytes", "Heap bytes allocated and still in use.", "gauge")
	fmt.Fprintf(&b, "%sgo_heap_alloc_bytes %d\n", namespace, mem.HeapAlloc)

	writeFamily(&b, "go_gc_cycles_total", "Completed garbage collection cycles.", "counter")
	fmt.Fprintf(&b, "%sgo_gc_cycles_total %d\n", namespace, mem.NumGC)

	_, _ = io.WriteString(w, b.String())
}

func writeFamily(b *strings.Builder, name, help, kind string) {
	fmt.Fprintf(b, "# HELP %s%s %s\n", namespace, name, help)
	fmt.Fprintf(b, "# TYPE %s%s %s\n", namespace, name, kind)
}

func sortedRequestKeys(m map[requestKey]uint64) []requestKey {
	keys := make([]requestKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})
	return keys
}

func sortedRouteKeys(m map[routeKey]histogram) []routeKey {
	keys := make([]routeKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		return keys[i].method < keys[j].method
	})
	return keys
}

// quote renders a label value with the escaping the exposition format requires:
// backslash, double quote and newline, and nothing else. Route patterns carry
// braces (`/sessions/{sessionId}`) which need no escaping, but the version
// string is stamped in at build time and a stray quote there would otherwise
// produce a page Prometheus rejects wholesale.
func quote(v string) string {
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// formatFloat renders a float the way the exposition format wants it: shortest
// representation that round-trips, and no exponent for the bucket bounds, which
// are compared as strings by anything reading the page.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
