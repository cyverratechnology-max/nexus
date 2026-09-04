# Security Baseline

Enrollment tokens are single-use, hashed at rest, and expire after 24 hours. Device credentials are unique and hashed at rest. Human sessions are signed, expire after eight hours, and derive organization scope server-side. Device listing and enrollment-token creation enforce organization scope and role checks.

Audited command execution is now available to platform administrators, organization administrators, and technicians. Commands are limited to 4096 characters, queued per device, executed by the enrolled agent with a 60-second timeout, and recorded in the command/audit tables. This is a privileged feature: do not expose the development configuration publicly, and add approval workflows and command allowlists before production use.

Remote desktop session lifecycle is implemented with RBAC, organization ownership, expiry, close controls, and audit records. The WebRTC signaling gateway, Windows interactive capture/input process, clipboard data channel, and TURN deployment are still required before a screen can be shared. Remote file transfer, interactive remote terminal, software deployment, and policy enforcement are not complete. Agent self-update is implemented with HTTPS delivery, SHA-256 verification, staging, and restart behavior; production releases should additionally sign binaries and manifests before distributing them.
