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

# Inside the container's own network namespace the API must listen on all
# interfaces; which host ports are exposed is controlled by docker-compose.
ENV PONY_ADDR=0.0.0.0:8080
EXPOSE 8080

ENTRYPOINT ["/app/pony", "serve"]
