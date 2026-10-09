FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ui.html ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ui .

FROM alpine:3.21
RUN adduser -D -H -u 10001 appuser
COPY --from=build /out/ui /usr/local/bin/ui
USER appuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ui"]
