# Console: builds the React SPA and the Go gateway, serving the SPA from /app/web/console.

FROM node:20-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS build
WORKDIR /src/console
COPY console/go.mod console/go.sum* ./
RUN go mod download
COPY console/ ./
RUN CGO_ENABLED=0 go build -o /out/console ./cmd/console

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 console
WORKDIR /app
COPY --from=build /out/console /app/console
COPY --from=web-build /src/console/web/console /app/web/console
USER 10001:10001
EXPOSE 8090
ENTRYPOINT ["/app/console"]