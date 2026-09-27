FROM golang:1.23-alpine AS build

RUN apk add --no-cache protobuf git

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

WORKDIR /src

COPY . .

RUN protoc \
    --go_out=. \
    --go_opt=module=github.com/likhitha281/forge \
    --go-grpc_out=. \
    --go-grpc_opt=module=github.com/likhitha281/forge \
    proto/forge.proto

RUN go mod tidy && \
    CGO_ENABLED=0 go build -o /coordinator ./cmd/coordinator && \
    CGO_ENABLED=0 go build -o /worker ./cmd/worker && \
    CGO_ENABLED=0 go build -o /checkpointable ./workloads/checkpointable && \
    CGO_ENABLED=0 go build -o /elastic ./workloads/elastic

FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl

RUN mkdir -p /etc/forge

COPY --from=build /coordinator /coordinator
COPY --from=build /worker /worker
COPY --from=build /checkpointable /usr/local/bin/checkpointable
COPY --from=build /elastic /usr/local/bin/elastic

COPY --from=build \
    /src/results/transition_costs_combined.csv \
    /etc/forge/transition_costs.csv

ARG TARGET=coordinator

RUN if [ "$TARGET" = "worker" ]; then \
        cp /worker /app; \
    else \
        cp /coordinator /app; \
    fi

ENTRYPOINT ["/app"]