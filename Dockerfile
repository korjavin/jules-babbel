# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install CGO dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go ./

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -o main .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

# Create a non-root user
RUN adduser -D -s /bin/sh appuser

# Create app directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Create static directory and copy frontend files
COPY index.html app.js privacy.html favicon.svg favicon-32x32.svg ./static/

# Copy migrations
COPY migrations ./migrations

# Make the binary executable and change ownership
RUN chmod +x ./main && chown -R appuser:appuser /app

# Create and set permissions for the data directory
RUN mkdir -p /app/data && chown -R appuser:appuser /app/data

USER appuser

# Expose port
EXPOSE 8080

# Set environment variables
ENV PORT=8080

# Run the application
CMD ["./main"]