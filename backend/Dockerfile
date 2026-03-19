FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o api ./cmd/api/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/api /api
ENTRYPOINT ["/api"]
