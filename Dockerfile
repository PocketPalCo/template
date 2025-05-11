FROM golang:1.24 as builder

WORKDIR /app
COPY . .
RUN go mod download &&  go build -o ./template-service ./cmd/main.go

FROM golang:1.24
COPY --from=builder /app/template-service /app/template-service
WORKDIR /app

ENTRYPOINT ["./template-service"]
