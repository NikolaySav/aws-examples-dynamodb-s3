# Stage 1: Build the application
FROM golang:1.23 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the entire project directory
COPY . .

# Set the working directory to the application entry point
WORKDIR /app/cmd

# Build the application (statically linked binary)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /app/main .

# Stage 2: Create the runtime container
FROM alpine:latest

# Install certificates for HTTPS if required (e.g., AWS SDK)
RUN apk add --no-cache ca-certificates

# Set the working directory inside the runtime container
WORKDIR /root/

# Copy the built binary from the builder stage
COPY --from=builder /app/main .

# Ensure the binary is executable
RUN chmod +x ./main

# Expose the application's port
EXPOSE 8080

# Command to run the application
CMD ["./main"]