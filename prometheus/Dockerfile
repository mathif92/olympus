# Syntax: docker/dockerfile:1

# --- build stage ---
FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/prometheus ./cmd/app

# --- runtime stage ---
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata docker-cli && \
    adduser -D -u 10001 prometheus

WORKDIR /app
COPY --from=build /out/prometheus /app/prometheus
# goose migration files are mounted from this dir at startup.
COPY migrations /app/migrations

# The Docker executor shells out to the docker CLI to reach the daemon on the
# host; run the service with the daemon socket mounted for real execution.
USER 10001:10001
EXPOSE 8092

ENTRYPOINT ["/app/prometheus"]
