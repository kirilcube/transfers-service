FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o transfers-api ./cmd/api
RUN go build -o mock-partner-bank-adapter ./cmd/mock_partner_bank_adapter


FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/transfers-api .
COPY --from=builder /app/mock-partner-bank-adapter .