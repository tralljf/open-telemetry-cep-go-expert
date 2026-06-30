FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/app ./cmd/${SERVICE}

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/app /app

EXPOSE 8080

ENTRYPOINT ["/app"]
