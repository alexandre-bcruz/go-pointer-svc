# Build
FROM golang:1.22 AS build
WORKDIR /app
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api

# Runtime
FROM alpine:3.20
ENV REDIS_ADDR=redis:6379
COPY --from=build /api /api
EXPOSE 3002
ENTRYPOINT ["/api"]
