# API Contract

Base path: `/api/v1`. JSON errors use `{ "error": { "message": "..." } }`.

## Human session

`POST /auth/login` accepts `{ "email": "...", "password": "..." }` and returns a signed bearer token plus `organization_id`.

`GET /devices` requires `Authorization: Bearer <session-token>` and returns `{ "data": [...] }` scoped to the token organization.

`POST /enrollment-tokens` requires an organization administrator, platform administrator, or technician session and returns a single-use token valid for 24 hours.

## Agent session

`POST /agent/enroll` accepts the enrollment token and device identity fields. It returns a device ID and unique credential. Store the credential with file permissions restricted to the service account.

`POST /agent/heartbeat` requires the device credential as a bearer token. It updates `last_seen`, status, hostname, and agent version. Production deployments must use HTTPS.
