# Stage 1: Build the Go binary
FROM golang:1.23-alpine AS builder

ENV GOTOOLCHAIN=auto
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Build static assets if needed (or embed via go:embed)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o server ./cmd/server

# Stage 2: Runtime image
FROM scratch

COPY --from=builder /build/server /server
COPY --from=builder /build/static /static

EXPOSE 8080
ENTRYPOINT ["/server"]