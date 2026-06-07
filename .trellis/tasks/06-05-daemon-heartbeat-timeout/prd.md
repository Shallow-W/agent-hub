# Fix daemon heartbeat offline timeout

## Goal

Prevent connected daemon machines from being marked offline before the first WebSocket ping/pong heartbeat, so @mentioned local agents can receive and complete chat tasks reliably.

## What I Already Know

* The backend `MachineTracker` currently uses a 15 second offline threshold.
* The daemon client sends WebSocket pings every 30 seconds.
* The backend server ping loop also runs every 30 seconds.
* Logs show machines being marked offline about 18-20 seconds after registration, before the next heartbeat.
* User messages sent after the premature offline mark persist successfully but do not get a Claude Code response.

## Requirements

* A connected daemon must remain online between registration and the first normal heartbeat.
* The offline threshold must be longer than the configured daemon/server ping cadence.
* The fix should be small and local to backend daemon liveness tracking unless verification reveals another required change.

## Acceptance Criteria

* [x] After daemon registration, the machine is not marked offline within the first 30 seconds.
* [x] Agent records remain online long enough for @mention dispatch to reach the daemon.
* [x] Existing backend tests pass, or any relevant test failure is explained.
* [x] Manual verification confirms daemon status remains connected past the old 15 second failure window.

## Definition of Done

* Fix implemented with minimal scope.
* Relevant tests run.
* Local daemon restarted and status verified.

## Out of Scope

* Reworking the daemon WebSocket protocol.
* Changing frontend mention UX.
* Changing Claude Code CLI invocation behavior.

## Technical Notes

* Likely file: `src/backend/internal/service/machine_tracker.go`.
* Related constants observed: `machineOfflineThreshold = 15 * time.Second`, `machineSweepInterval = 30 * time.Second`.
* Daemon constants observed: `WS_PING_INTERVAL_MS = 30000`, `INBOUND_WATCHDOG_MS = 70000`.
* Implemented by increasing `machineOfflineThreshold` to 75 seconds and adding MachineTracker regression tests.
* Verified with `go test ./...` under `src/backend`.
