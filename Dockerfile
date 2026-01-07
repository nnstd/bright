FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build argument for version
ARG VERSION=dev

# Build the application with version
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-X main.Version=${VERSION}" -o search-db .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/search-db .

# Set executable permissions
RUN chmod +x ./search-db

# Create data directory
RUN mkdir -p /root/data

# Expose port
EXPOSE 3000

# Run the application
CMD ["./search-db"]
