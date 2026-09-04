# Agent

The native Go agent runs without a language runtime on the endpoint. First run requires `--enrollment-token`; subsequent runs read the credential from the OS user configuration directory. It sends a real hostname, OS, architecture, and agent version during enrollment, then sends a heartbeat every 30 seconds.

Build with `go build -o bin/cyverra-agent ./agent/cmd/agent`. Windows service and Linux systemd wrappers should invoke the binary with a managed state path and an installer-provided enrollment token. Credentials must never be passed in shell history in production installers.
