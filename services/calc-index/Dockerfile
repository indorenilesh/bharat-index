FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/calc-index .

FROM alpine:3.21
RUN adduser -D -H -u 10001 appuser
COPY --from=build /out/calc-index /usr/local/bin/calc-index
USER appuser
ENTRYPOINT ["/usr/local/bin/calc-index"]
