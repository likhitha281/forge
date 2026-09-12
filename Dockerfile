FROM golang:1.23-alpine AS build
RUN apk add --no-cache protobuf git
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
WORKDIR /src
COPY . .
RUN protoc --go_out=. --go_opt=module=github.com/likhitha281/forge --go-grpc_out=. --go-grpc_opt=module=github.com/likhitha281/forge proto/forge.proto
RUN go mod tidy && CGO_ENABLED=0 go build -o /coordinator ./cmd/coordinator && CGO_ENABLED=0 go build -o /worker ./cmd/worker
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
COPY --from=build /coordinator /coordinator
COPY --from=build /worker /worker
ARG TARGET=coordinator
RUN if [ "$TARGET" = "worker" ]; then cp /worker /app; else cp /coordinator /app; fi
ENTRYPOINT ["/app"]
