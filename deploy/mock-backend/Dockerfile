# syntax=docker/dockerfile:1
FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /mock-backend ./deploy/mock-backend

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /mock-backend /mock-backend
ENTRYPOINT ["/mock-backend"]
