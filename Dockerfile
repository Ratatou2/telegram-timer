# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /telegram-timer .

# Run stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /telegram-timer .
# Default DB path when using volume mount at /data
ENV DB_PATH=/data/reminders.db
EXPOSE 8080
ENTRYPOINT ["./telegram-timer"]
