.PHONY: test api agent frontend

test:
	go test ./backend/... ./agent/...

api:
	go run ./backend/cmd/api

agent:
	go run ./agent/cmd/agent

frontend:
	cd frontend && npm run dev
