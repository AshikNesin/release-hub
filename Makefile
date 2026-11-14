.PHONY: build web go clean test dev run

# One binary: the React UI (web/) is built first, then embedded by go:embed
# (web/embed.go). srv/ is a package directory, so -o srv writes srv/srv.
build: web go

web:
	pnpm --dir web install --frozen-lockfile
	pnpm --dir web build

go:
	go build -o srv ./cmd/srv

clean:
	rm -f srv/srv
	find web/dist -mindepth 1 ! -name .gitkeep -exec rm -rf {} +

test:
	go test ./...
	pnpm --dir web typecheck

# Development: Go API on :9100 and Vite (HMR) on :5173, which proxies the API.
# Open http://localhost:5173.
run:
	go run ./cmd/srv -listen :9100 -db db.sqlite3 -artifacts artifacts -base-url http://localhost:9100

dev:
	@trap 'kill 0' INT TERM; \
	$(MAKE) --no-print-directory run & \
	pnpm --dir web dev & \
	wait
