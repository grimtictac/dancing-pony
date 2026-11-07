FROM golang:1.27-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/pony ./cmd/pony

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /out/pony /app/pony

EXPOSE 8080

ENTRYPOINT ["/app/pony", "serve"]
