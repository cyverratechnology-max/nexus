# Architecture

## Decision

The empty workspace is bootstrapped as a modular Go backend and native Go agent with a React/Vite frontend. PostgreSQL owns transactional state. Redis/NATS, metrics storage, object storage, and independent remote subsystems will be introduced behind interfaces as each phase becomes functional.

## Boundaries

- `backend`: versioned REST API, authentication, tenant authorization, migrations, audit records, and device lifecycle.
- `agent`: native endpoint service with identity, HTTPS enrollment, inventory, heartbeat, and reconnect behavior.
- `frontend`: dashboard client that consumes the REST contract.
- `infrastructure`: local PostgreSQL and future observability dependencies.

## Security model

Tenant-owned records carry `organization_id`. API handlers derive organization scope from the authenticated session or enrollment token; clients cannot select a tenant scope. Enrollment tokens are stored hashed and are single-use. Device credentials are random per-device secrets, stored hashed by the server, and sent only over HTTPS in production. All privileged actions will require RBAC and audit logging.

## Delivery phases

1. Core identity, organizations, devices, enrollment, heartbeat, and audit.
2. Inventory and time-series metrics with alert evaluation.
3. Policies, compliance, scripts, commands, software, and patches.
4. WebSocket events, workers, notifications, and automation.
5. Remote terminal/files and separately reviewed WebRTC desktop subsystem.
6. Security events, vulnerabilities, observability, packaging, and horizontal scaling.
