# Observability

The API reports what it is doing in Prometheus text format. Until this existed the
only signal a running deployment gave was `/health` — up or not up — which cannot
tell a slow endpoint from a fast one, or a pod serving 404s from a pod serving
nothing at all.

## Where it is

**A separate listener from the API**, on `METRICS_PORT` (default `9090`), path
`/metrics`. Set `METRICS_PORT=off` to disable it.

```sh
curl -s localhost:9090/metrics
```

The separation is the security control. Traefik routes the public hostname at the
API's port, so anything mounted there is reachable from the internet — and this
page names every route, counts every error and reports the traffic volume of a
household. On its own port it is reachable from inside the cluster only, which is
where the scraper lives, and that holds without anyone having to get an ingress
exclusion right.

It follows that **the Deployment needs the port declared** before Prometheus can
find it. That manifest lives in the GitOps repo, not here:

```yaml
ports:
  - name: http
    containerPort: 8080
  - name: metrics
    containerPort: 9090
```

with whatever your Prometheus uses to discover targets — a `ServiceMonitor`, or
the `prometheus.io/scrape`, `prometheus.io/port: "9090"` annotation pair.

## What is exposed

Every metric is prefixed `iron_temple_`, the runtime ones included. One prefix
rather than client_golang's bare `go_*` names: nothing else is exported from this
process, and an unambiguous prefix means adopting a real client library later
cannot collide with what is already being scraped.

| Metric | Type | Labels |
|--|--|--|
| `iron_temple_http_requests_total` | counter | `method`, `route`, `status` |
| `iron_temple_http_request_duration_seconds` | histogram | `method`, `route` |
| `iron_temple_http_requests_in_flight` | gauge | — |
| `iron_temple_db_pool_connections` | gauge | `state` (acquired/idle/total) |
| `iron_temple_db_pool_max_connections` | gauge | — |
| `iron_temple_build_info` | gauge | `version`, `environment` |
| `iron_temple_process_start_time_seconds` | gauge | — |
| `iron_temple_go_goroutines` | gauge | — |
| `iron_temple_go_heap_alloc_bytes` | gauge | — |
| `iron_temple_go_gc_cycles_total` | counter | — |

`route` is the chi route **pattern** — `/api/v1/sessions/{sessionId}`, never
`/api/v1/sessions/412`. Labelling by path would mint a time series per session id
and the scrape would then grow for the life of the training history, which is the
classic way a metrics endpoint becomes the most expensive thing in a deployment.
Two layers stop it: chi reports its own `/api/v1/*` wildcard for a miss inside the
API, and a request that matched nothing at all is folded into a single
`route="unmatched"` series. `method` is folded the same way, into `other`, because
HTTP allows any token on the request line and an unbounded label is an unbounded
label whoever writes to it.

Latency is deliberately **not** split by status. A route's histogram answers "how
long does this take"; dividing it by outcome halves the sample count without
changing the answer.

## Queries worth having

```promql
# Request rate by route
sum by (route) (rate(iron_temple_http_requests_total[5m]))

# Error rate
sum(rate(iron_temple_http_requests_total{status=~"5.."}[5m]))
  / sum(rate(iron_temple_http_requests_total[5m]))

# p95 latency by route
histogram_quantile(0.95,
  sum by (le, route) (rate(iron_temple_http_request_duration_seconds_bucket[5m])))

# Pool headroom — the resource this deployment is most likely to exhaust
iron_temple_db_pool_connections{state="acquired"} / iron_temple_db_pool_max_connections

# Restarts, without needing kube-state-metrics
changes(iron_temple_process_start_time_seconds[1h])
```

## Why it is hand-rolled

`internal/metrics` writes the exposition format itself, in about 200 lines of
stdlib. The Go module is vendored and `prometheus/client_golang` is not in the
tree; pulling it in to count requests would add a dependency graph an order of
magnitude larger than the thing it measures, and the format is a documented text
protocol with a fixed, small metric set here.

The package imports neither chi nor pgx. The route pattern arrives as a string
from a middleware in `internal/api` that already has chi, and pool statistics
arrive through a callback the server registers — so the numbers stay separable
from what produces them, and the package is testable without a router or a
database.

## Tests

`internal/metrics/metrics_test.go` covers the registry and the rendering,
including the escaping and the cardinality folds; run it with `-race`, which is
what asserts the whole registry is guarded while a scrape reads it.

`internal/api/metrics_integration_test.go` covers the part only a real router can
prove — that the label is the pattern and never the path. It reads the live
registry rather than an HTTP endpoint, because there deliberately isn't one on
that router. Counters are cumulative across the suite, so every assertion there
is on a delta around a known call.
