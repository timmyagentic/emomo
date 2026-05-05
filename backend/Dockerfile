# Build stage
FROM golang:1.26.2-alpine AS builder

WORKDIR /app

# Install git and build tools (gcc needed for CGO/SQLite)
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the API binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o api ./cmd/api

# Final stage
FROM alpine:latest

# Install required packages
# findutils provides GNU find which has better Unicode support than busybox find
RUN apk --no-cache add ca-certificates tzdata findutils

# Set UTF-8 locale for proper handling of Chinese/Unicode filenames
# Alpine uses musl which has limited locale support, but C.UTF-8 works for file operations
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
ENV LANGUAGE=C.UTF-8

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/api .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/scripts ./scripts

# Create local static image data directory
RUN mkdir -p ./data/memes && \
    chmod +x ./scripts/*.sh 2>/dev/null || true

# Expose port (Hugging Face Spaces uses 7860 by default)
EXPOSE 7860

# Set default port for Hugging Face Spaces
ENV PORT=7860

# Run the binary (with directory check)
CMD ["sh", "-c", "./scripts/check-data-dir.sh && ./api"]
