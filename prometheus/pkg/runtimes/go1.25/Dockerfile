FROM golang:1.25-alpine AS build
WORKDIR /app
COPY . /app
RUN CGO_ENABLED=0 go build -trimpath -o /out/handler .

FROM alpine:3.20
COPY --from=build /out/handler /handler
ENTRYPOINT ["/handler"]
