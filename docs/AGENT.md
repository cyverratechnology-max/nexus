# Agent

The native Go agent runs without a language runtime on the endpoint. First run requires `--enrollment-token`; subsequent runs read the credential from the OS user configuration directory. It sends a real hostname, OS, architecture, and agent version during enrollment, then sends a heartbeat every 30 seconds.

The agent checks `/downloads/agent-manifest.json` at startup and every heartbeat cycle. When a newer semantic version is available, it downloads the platform binary, verifies the published SHA-256, stages it, and restarts. Linux replaces the executable atomically; Windows uses a temporary `.cmd` helper because Windows locks the running executable. If download, checksum, or replacement fails, the current agent continues running.

Build with `go build -o bin/cyverra-agent ./agent/cmd/agent`. Windows service and Linux systemd wrappers should invoke the binary with a managed state path and an installer-provided enrollment token. Credentials must never be passed in shell history in production installers.
