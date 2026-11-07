FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/the-dancing-pony-api .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /out/the-dancing-pony-api /app/the-dancing-pony-api

EXPOSE 8080

ENTRYPOINT ["/app/the-dancing-pony-api"]
