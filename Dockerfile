FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o webhook ./cmd/webhook

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/webhook /webhook
ENTRYPOINT ["/webhook"]
