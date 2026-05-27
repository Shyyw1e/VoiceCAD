FROM golang:1.24.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/voicecad ./cmd/voicecad

FROM alpine:3.21

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 voicecad

WORKDIR /app
COPY --from=builder /out/voicecad /app/voicecad

RUN mkdir -p /app/data/storage && chown -R voicecad:voicecad /app

USER voicecad
EXPOSE 8080

ENTRYPOINT ["/app/voicecad"]
