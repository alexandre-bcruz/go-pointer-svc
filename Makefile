.PHONY: run dev docker-up
run:
	REDIS_ADDR=localhost:6379 go run ./cmd/api

dev:
	air || true

docker-up:
	docker compose up --build


lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	go fmt ./...

build:
	go build ./...

ci: fmt vet lint build

build-linux:
	GOOS=linux GOARCH=amd64 go build -o pointer-svc-app cmd/api/main.go