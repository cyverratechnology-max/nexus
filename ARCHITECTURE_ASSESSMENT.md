# UEM/RMM Architecture Assessment

## Scope and Finding

The workspace was inspected on 2026-09-03. It contains no files, no project manifest, no database schema, no source code, and no Git metadata. Therefore there is no existing UEM implementation available to analyze, preserve, refactor, or migrate.

The sections below distinguish verified facts from the proposed bootstrap architecture. They are not claims about an existing application.

## CURRENT ARCHITECTURE

- Verified: no application files are present.
- Language/framework: unknown; no manifest exists.
- Runtime/deployment model: unknown; no Docker, web server, or service configuration exists.
- Service boundaries: none identifiable.
- Scalability characteristics: cannot be assessed.

## CURRENT FEATURES

No existing features can be verified. In particular, there is no evidence of device enrollment, an endpoint agent, monitoring, policy management, remote management, automation, authentication, or a dashboard.

## CURRENT DATABASE

No schema, migration, ORM configuration, connection configuration, or seed data exists. PostgreSQL is recommended for the bootstrap implementation, with a time-series store for high-volume metrics and object storage for installers, artifacts, and large files.

## CURRENT API

No API routes, handlers, OpenAPI specification, authentication middleware, WebSocket gateway, or worker interface exists.

## CURRENT AGENT

No endpoint agent, installer, service definition, enrollment flow, device identity mechanism, heartbeat, inventory collector, or update mechanism exists.

## CURRENT FRONTEND

No frontend source, build configuration, routes, design system, or dashboard exists.

## CURRENT SECURITY

No authentication, authorization, tenant isolation, secret management, TLS configuration, audit logging, input validation, rate limiting, or command execution controls exist. A security baseline must be established before exposing any management endpoint.

## CURRENT LIMITATIONS

1. There is no source of truth from which to preserve working behavior.
2. There is no schema to migrate or data to back up.
3. The requested platform is a multi-service product, not a safe single-file implementation.
4. Endpoint management and remote-control features require platform-specific agent code, signing, privileged installation, and security review.
5. The requested scale requires operational infrastructure and tests that cannot be inferred from an empty directory.

## PROPOSED ARCHITECTURE

Bootstrap a modular monorepo with these deployable boundaries:

- `apps/web`: React and TypeScript dashboard.
- `services/api`: versioned REST API, validation, RBAC, tenant context, and audit logging.
- `services/realtime`: authenticated WebSocket gateway for device events and command state.
- `services/worker`: asynchronous commands, notifications, inventory processing, and deployments.
- `services/scheduler`: heartbeats, monitoring evaluation, maintenance windows, and automation triggers.
- `agents/windows` and `agents/linux`: signed background services with identity, enrollment, heartbeat, inventory, monitoring, command, policy, update, and local queue modules.
- PostgreSQL: transactional entities and tenant-scoped relational data.
- Redis or NATS: queues, event delivery, locks, and transient connection state.
- TimescaleDB or another PostgreSQL-compatible time-series layer: metrics.
- S3/MinIO: installers, software packages, agent releases, and streamed file artifacts.
- Reverse proxy/WAF: TLS termination, request limits, and routing.

Every tenant-owned table should carry `organization_id`, use foreign keys and indexes, and enforce authorization in application code. PostgreSQL Row Level Security should be added after tenant context propagation is tested. Remote actions must require explicit permissions, device authorization, timeouts, and append-only audit records.

## CORE DATA MODEL

Initial migrations should cover:

- Organizations, sites, users, roles, permissions, and sessions.
- Devices, enrollment tokens, agent identities, groups, and group membership.
- Hardware/software/network inventory.
- Metrics, monitoring rules, alerts, and alert transitions.
- Policies, immutable policy versions, assignments, effective policy snapshots, and compliance.
- Scripts, script versions, command jobs, and execution output.
- Applications, packages, deployment jobs, patches, and maintenance windows.
- Automation rules/runs, notifications, remote sessions, security events, vulnerabilities, and audit logs.

Use UUID primary keys, UTC timestamps, explicit status enums, uniqueness constraints, soft deletion only where operationally useful, and JSONB only for genuinely extensible configuration.

## API AND EVENT PLAN

Start with `/api/v1` and consistent envelopes for success, pagination, and validation errors. First resources should be `auth`, `organizations`, `sites`, `users`, `devices`, `groups`, and `audit`. Add monitoring, alerts, policies, scripts, deployments, patches, automation, security, and remote operations behind the same authorization and tenant middleware.

Use structured events such as `device.online`, `device.offline`, `device.heartbeat`, `command.created`, `command.completed`, `alert.triggered`, `alert.resolved`, and `policy.applied`. WebSocket subscriptions must be authorized per organization and resource; event payloads must never be treated as authorization claims.

## MIGRATION PLAN

Because no existing code or data exists, this is a bootstrap rather than a migration:

1. Establish repository layout, language choices, formatting, linting, and CI.
2. Add local Docker infrastructure, environment templates, health checks, and secret boundaries.
3. Implement PostgreSQL migrations for organizations, identity, RBAC, devices, enrollment, and audit logs.
4. Implement secure authentication, sessions/refresh-token rotation, lockout, MFA-ready interfaces, and tenant authorization.
5. Implement device enrollment, unique device credentials, heartbeat, inventory, and agent reconnect behavior.
6. Add metrics ingestion, retention, monitoring rules, alerts, dashboards, and asynchronous workers.
7. Add policy versioning/compliance, scripts/commands, software and patch workflows.
8. Add remote terminal and file operations with approval and audit controls.
9. Add remote desktop signaling/WebRTC as a separately reviewed subsystem.
10. Add security and vulnerability data models, automation, notifications, observability, and production deployment.
11. Run unit, integration, tenant-isolation, authorization, agent, and security tests before enabling privileged features.

Each phase should ship with migrations, API contracts, focused tests, and rollback notes. Remote desktop, arbitrary script execution, file deletion, and agent self-update should remain disabled by default until their security controls and signing pipeline are verified.

## BLOCKER TO IMPLEMENTATION

The requested transformation cannot be performed against this workspace because the existing UEM codebase is absent. Provide or open the actual project directory, or explicitly authorize a new-platform bootstrap and select the implementation stack (for example, TypeScript/NestJS + React or Python/FastAPI + React). Once the source is available, the assessment can be expanded with file-level findings and implementation can proceed incrementally without inventing existing behavior.
