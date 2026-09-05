FROM golang:1.26.8-alpine3.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && CGO_ENABLED=0 go build -o /out/worker ./cmd/worker
FROM alpine:3.23.5
RUN apk add --no-cache ca-certificates tzdata && adduser -D app
COPY --from=build /out/api /out/worker /app/
USER app
EXPOSE 8080
CMD ["/app/api"]
