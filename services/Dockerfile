FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/market-service ./cmd/market-service

FROM alpine:3.21

RUN adduser -D -H -u 10001 appuser
COPY --from=build /out/market-service /usr/local/bin/market-service

USER appuser
ENTRYPOINT ["/usr/local/bin/market-service"]
