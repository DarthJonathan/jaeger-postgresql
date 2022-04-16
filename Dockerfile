### ---------------------- ###
# Stage 1 Build the plugin ###
### ---------------------- ###
FROM golang:1.15-buster AS builder
WORKDIR /app

COPY . ./

RUN CGO_ENABLED=0 go build -o ./build/jaeger-postgresql ./cmd/jaeger-pg-store/

### ---------------------- ###
# Stage 2 Serve the plugin ###
### ---------------------- ###
FROM debian:buster

COPY --from=builder /app/build/jaeger-postgresql /go/bin/jaeger-postgresql

RUN mkdir /plugin

# /plugin/ location is defined in jaeger-operator
CMD ["cp", "/go/bin/jaeger-postgresql", "/plugin/jaeger-postgresql"]