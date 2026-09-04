# API Contract

Base path: `/api/v1`. JSON errors use `{ "error": { "message": "..." } }`.

## Human session

`POST /auth/login` accepts `{ "email": "...", "password": "..." }` and returns a signed bearer token plus `organization_id`.

`GET /devices` requires `Authorization: Bearer <session-token>` and returns `{ "data": [...] }` scoped to the token organization.

`POST /enrollment-tokens` requires an organization administrator, platform administrator, or technician session and returns a single-use token valid for 24 hours.

## Agent session

`POST /agent/enroll` accepts the enrollment token and device identity fields. It returns a device ID and unique credential. Store the credential with file permissions restricted to the service account.

`POST /agent/heartbeat` requires the device credential as a bearer token. It updates `last_seen`, status, hostname, and agent version. Production deployments must use HTTPS.

`POST /agent/inventory` requires the device credential and stores the latest real host inventory (OS, architecture, CPU cores, memory, and network interfaces).

`POST /agent/metrics` requires the device credential and appends CPU and memory samples. Linux agents collect CPU from `/proc/stat` and memory from `/proc/meminfo`.

`GET /monitoring/summary` requires a human session and returns the latest metric sample for each device in the current organization.

`GET /devices/{id}/metrics` and `GET /devices/{id}/inventory` require a human session and enforce organization ownership server-side.

`POST /devices/{id}/commands` queues an audited command for a device. Only platform administrators, organization administrators, and technicians may create commands. The agent polls `GET /agent/commands`, executes one command with a 60-second timeout using the host shell, and posts the result to `POST /agent/commands/{id}/result`.
