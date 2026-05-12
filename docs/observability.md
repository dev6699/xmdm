# Observability

XMDM exposes a small built-in observability surface for operators.

## HTTP request logging

Every server request is logged with:

- request ID
- trace ID
- method
- normalized route
- status code
- request duration

The server also returns:

- `X-Request-Id`
- `traceparent`

If the request already carries a valid `traceparent`, the trace ID is preserved and a new span ID is issued for the response.

## Metrics

The server exposes Prometheus-style metrics on `GET /metrics`.

Current signals include:

- `xmdm_http_requests_total`
- `xmdm_http_request_duration_seconds_bucket`
- `xmdm_http_request_duration_seconds_sum`
- `xmdm_http_request_duration_seconds_count`

Route labels are normalized to avoid high-cardinality device and resource IDs.

## What to inspect

For core production flows, start with:

- request logs for a specific request ID or trace ID
- `/metrics` for request counts and latency
- the `traceparent` response header to correlate a client request with the server log entry
