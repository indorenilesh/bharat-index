FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go admin.html ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/admin .

FROM alpine:3.21
RUN adduser -D -H -u 10001 appuser
COPY --from=build /out/admin /usr/local/bin/admin
USER appuser
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/admin"]
