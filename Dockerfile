FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates su-exec

# Create non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Create required directories and set ownership
RUN mkdir -p /app/uploads /app/certs && \
    chown -R appuser:appgroup /app

WORKDIR /app

# Copy binary and entrypoint from builder/host
COPY --from=builder /app/main .
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh && chown appuser:appgroup /app/main

# Entrypoint runs as root, fixes bind-mount ownership, then drops to appuser
EXPOSE 8080

ENTRYPOINT ["./entrypoint.sh"]
CMD ["./main"]