FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o acp-posts .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/acp-posts .
RUN mkdir -p /data
ENV SLACK_BOT_TOKEN=""
ENV SLACK_CHANNEL_ID=""
ENV SLACK_THREAD_TS=""
CMD ["./acp-posts"]
