# Syntax: docker/dockerfile:1

# --- build stage ---
FROM golang:1.26-alpine AS build
WORKDIR /src

# authz is a local replace target (go.mod: replace ... => ../authz)
COPY authz/ /authz/
COPY prometheus/go.mod prometheus/go.sum ./
RUN go mod download

COPY prometheus/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/prometheus ./cmd/app

# --- runtime stage ---
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 prometheus

WORKDIR /app
COPY --from=build /out/prometheus /app/prometheus
# goose migration files are mounted from this dir at startup.
COPY prometheus/migrations /app/migrations

USER 10001:10001
EXPOSE 8092

ENTRYPOINT ["/app/prometheus"]
