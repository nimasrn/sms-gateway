FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go files
COPY main.go .

# Initialize go module
RUN go mod init loadtest && \
    go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o loadtest .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/loadtest .

# Run the load test
CMD ["./loadtest"]
