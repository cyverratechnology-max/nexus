# Security Baseline

Enrollment tokens are single-use, hashed at rest, and expire after 24 hours. Device credentials are unique and hashed at rest. Human sessions are signed, expire after eight hours, and derive organization scope server-side. Device listing and enrollment-token creation enforce organization scope and role checks.

This first vertical slice does not yet expose arbitrary command execution, remote terminal, file operations, remote desktop, software deployment, policy enforcement, or self-update. Those features must be added only with explicit authorization, timeout, audit, path validation, package/signature verification, and security tests. Never run this development configuration with public network access.
