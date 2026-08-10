# Syntax: docker/dockerfile:1

# --- build stage ---
FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mneme ./cmd/app

# --- runtime stage ---
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 mneme

WORKDIR /app
COPY --from=build /out/mneme /app/mneme
# goose migration files are mounted from this dir at startup.
COPY migrations /app/migrations

USER 10001:10001
EXPOSE 8088

ENTRYPOINT ["/app/mneme"]