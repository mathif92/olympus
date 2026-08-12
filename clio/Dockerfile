# Syntax: docker/dockerfile:1

# --- build stage ---
FROM golang:1.26-alpine AS build
WORKDIR /src

# authz is a local replace target (go.mod: replace ... => ../authz)
COPY authz/ /authz/
COPY clio/go.mod clio/go.sum ./
RUN go mod download

COPY clio/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/clio ./cmd/app

# --- runtime stage ---
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 clio

WORKDIR /app
COPY --from=build /out/clio /app/clio
# goose migration files are mounted from this dir at startup.
COPY clio/migrations /app/migrations

USER 10001:10001
EXPOSE 8087

ENTRYPOINT ["/app/clio"]
