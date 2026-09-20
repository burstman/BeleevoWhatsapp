.PHONY: dev build styles templ db-up db-reset migrate run-worker test tidy

# --- Development ---
dev:
	air -c .air.toml

build:
	go mod tidy
	templ generate
	go build -o bin/api ./cmd/app
	go build -o bin/worker ./cmd/worker
	go build -o bin/migrate ./cmd/scripts/migrate

styles:
	npm run styles

templ:
	templ generate

# --- Database (local PostgreSQL) ---
db-up: migrate

migrate:
	go run ./cmd/scripts/migrate

db-reset:
	dropdb --if-exists converty_whatsapp
	createdb converty_whatsapp
	go run ./cmd/scripts/migrate

# --- Worker ---
run-worker:
	go run ./cmd/worker

test:
	go test ./...

tidy:
	go mod tidy
	gofmt -w .