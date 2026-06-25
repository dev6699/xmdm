# System Architecture

## Architecture Summary

XMDM is a two-plane control system:

- an Android launcher running on each managed device
- a Go control plane that owns auth, policy, content, command delivery, audit,
  and the admin dashboard

The server owns policy truth. The launcher applies signed snapshots and reports
state back to the server.

## Architecture Decisions

- The admin dashboard is server-rendered and is the supported operator surface.
- Device policy and content state flow from the server to the launcher through
  signed config snapshots.
- PostgreSQL stores transactional state, relationships, audit, command state,
  telemetry, logs, and device info.
- Object storage stores binary artifacts referenced by server metadata.
- PostgreSQL command state is authoritative; MQTT is a push channel, and HTTP
  polling is the recovery path.
- Plugins are statically registered at startup.
- Core behavior must work when optional plugins are omitted.
- PostgreSQL access stays server-side.

The runtime topology lives in [../README.md#system-shape](../README.md#system-shape).

## Failure Domains

- If MQTT is unavailable, command polling must remain usable.
- If object storage is unavailable, metadata reads and admin auth should still be
  diagnosable.
- If optional plugins are omitted, core enrollment, sync, dashboard, and command
  delivery must continue.
- If a device is offline, the launcher preserves its last accepted local state
  and resumes sync when connectivity returns.
