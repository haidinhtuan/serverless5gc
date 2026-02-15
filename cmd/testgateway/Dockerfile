FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /testgateway ./cmd/testgateway/
FROM alpine:3.19
COPY --from=builder /testgateway /testgateway
EXPOSE 8080
CMD ["/testgateway"]
