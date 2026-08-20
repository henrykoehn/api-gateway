# syntax=docker/dockerfile:1
FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /gateway ./cmd/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /gateway /gateway
COPY configs/gateway.yaml /configs/gateway.yaml
EXPOSE 8080
ENTRYPOINT ["/gateway", "--config", "/configs/gateway.yaml"]
