# syntax=docker/dockerfile:1
FROM golang:1.24-alpine as builder
WORKDIR /build
ADD . .
RUN --mount=type=cache,target=/root/.cache/go-build go test ./... && CGO_ENABLED=0 go build -o ./bin/go-boilerplate ./main.go

FROM alpine
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/bin/go-boilerplate /app
EXPOSE 3311
ENTRYPOINT ["/app/go-boilerplate"]
