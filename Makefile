GO_IMAGE := golang:1.23-alpine
MODULE := github.com/likhitha281/forge
WORKERS ?= 3

.PHONY: help proto tidy fmt test race build build-windows up down restart ps logs worker-logs coordinator-logs db prometheus grafana clean

help:
	@echo "Forge - Distributed Task Execution Engine"
	@echo ""
	@echo "Development:"
	@echo "  make proto             Generate protobuf/gRPC code"
	@echo "  make tidy              Run go mod tidy"
	@echo "  make fmt               Format Go source"
	@echo "  make test              Run all Go tests"
	@echo "  make race              Run tests with Go race detector"
	@echo "  make build             Build Linux CLI"
	@echo "  make build-windows     Build forge.exe"
	@echo ""
	@echo "Docker:"
	@echo "  make up                Start Forge with 3 workers"
	@echo "  make down              Stop Forge"
	@echo "  make restart           Rebuild and restart Forge"
	@echo "  make ps                Show running services"
	@echo "  make logs              Follow all logs"
	@echo "  make worker-logs       Follow worker logs"
	@echo "  make coordinator-logs  Follow coordinator logs"
	@echo ""
	@echo "Infrastructure:"
	@echo "  make db                Open PostgreSQL shell"
	@echo "  make prometheus        Print Prometheus URL"
	@echo "  make grafana           Print Grafana URL"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean             Remove local binaries"

proto:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		$(GO_IMAGE) \
		sh -c "apk add --no-cache protobuf git && \
		go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1 && \
		go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 && \
		protoc \
		--go_out=. \
		--go_opt=module=$(MODULE) \
		--go-grpc_out=. \
		--go-grpc_opt=module=$(MODULE) \
		proto/forge.proto"

tidy:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		$(GO_IMAGE) \
		sh -c "apk add --no-cache git && go mod tidy"

fmt:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		$(GO_IMAGE) \
		gofmt -w cmd internal gen

test:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		$(GO_IMAGE) \
		go test ./...

race:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		-e CGO_ENABLED=1 \
		$(GO_IMAGE) \
		sh -c "apk add --no-cache gcc musl-dev && go test -race ./..."

build:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		-e CGO_ENABLED=0 \
		$(GO_IMAGE) \
		go build -o forge ./cmd/client

build-windows:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		-e GOOS=windows \
		-e GOARCH=amd64 \
		-e CGO_ENABLED=0 \
		$(GO_IMAGE) \
		go build -o forge.exe ./cmd/client

up:
	docker compose up -d --build --scale worker=$(WORKERS)

down:
	docker compose down

restart:
	docker compose down
	docker compose up -d --build --scale worker=$(WORKERS)

ps:
	docker compose ps

logs:
	docker compose logs -f

worker-logs:
	docker compose logs -f worker

coordinator-logs:
	docker compose logs -f coordinator

db:
	docker compose exec postgres psql -U forge -d forge

prometheus:
	@echo "Prometheus: http://localhost:9091"

grafana:
	@echo "Grafana: http://localhost:3000"

clean:
	rm -f forge forge.exe