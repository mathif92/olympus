FROM rust:1.80-alpine AS build
WORKDIR /app
COPY . /app
RUN cargo build --release

FROM alpine:3.20
COPY --from=build /app/target/release/handler /handler
ENTRYPOINT ["/handler"]
