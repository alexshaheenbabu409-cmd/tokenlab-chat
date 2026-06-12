FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod init tokenlab-chat && \
    go build -ldflags="-s -w" -o server .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
