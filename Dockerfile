# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Cài các tool cần thiết cho build (nếu cần)
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/main.go

# Run stage
FROM alpine:latest

WORKDIR /app

# Cần ca-certificates cho TLS, tzdata cho timezone DB
RUN apk --no-cache add ca-certificates tzdata

# Thiết timezone mặc định (có thể override qua env TZ trên Render)
ENV TZ=Asia/Ho_Chi_Minh

# Copy built binary
COPY --from=builder /app/server .

# Set runtime port for Render (Render cũng cung cấp PORT env)
ENV PORT=8080

EXPOSE 8080

CMD ["./server"]
