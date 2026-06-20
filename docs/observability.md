# Observability

XMDM exposes a small built-in observability surface for operators.

Use this page together with the dashboard pages in [Admin Dashboard](admin-dashboard.md) and the recovery procedures in [Disaster Recovery And Rollback](disaster-recovery-and-rollback.md).

`GET /health` is a plain liveness probe that returns `200 OK` when the server process is up.
Use the admin dashboard health strip to inspect PostgreSQL, object storage, and MQTT publish readiness.

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

- `GET /health` for server liveness
- the admin dashboard health strip for backend service readiness, including MQTT publish
- request logs for a specific request ID or trace ID
- `/metrics` for request counts and latency
- the `traceparent` response header to correlate a client request with the server log entry

## Concrete Lookup Examples

### Request ID Lookup

When a browser action fails, copy the `X-Request-Id` response header from the network inspector or the server response and search the server logs for that value.

That gives you the exact request path, status code, and duration for the failing action.

### Traceparent Lookup

If the client sends `traceparent`, the server keeps the trace ID and emits a new span ID for the response.

Use the `traceparent` response header to correlate a dashboard request, the matching server log line, and any downstream calls that carry the same trace ID.

### Command Latency

The server records command state transitions in the command row, and the dashboard command detail page shows the queued, sent, acked, failed, or expired state.

For a concrete latency check, compare the command `created_at` and acknowledgement time in the dashboard, then use `/metrics` to confirm request volume and server-side timing for the related admin and device endpoints.

### Command Failure Rate

Use the dashboard command list to count failed and expired rows for a specific time window.

Then confirm the HTTP request counters and duration histogram at `/metrics` to see whether the failure is clustered around command creation, device polling, or acknowledgement traffic.

### Stale Device Detection

The overview dashboard highlights stale active devices and commands waiting for acknowledgement.

Open the device detail page to confirm the latest device info, recent logs, and recent sync timing before deciding whether the device is offline or only delayed.

### MQTT Outage Diagnosis

If MQTT is unavailable, command publishing should fall back to the HTTP polling path and the queued command should remain visible in PostgreSQL.

Check the broker health, server logs for publish failures, the command detail transport source, and the device polling endpoint to confirm that the device can still fetch pending commands until MQTT recovers.

### Object Storage Artifact Failure Diagnosis

If app, file, certificate, or managed-file downloads fail, check the object storage service first and then compare the checksum or storage metadata in the dashboard detail pages.

Use the admin pages for the affected artifact type to confirm the upload exists, the latest version is the expected one, and the server can still fetch the blob from object storage.
