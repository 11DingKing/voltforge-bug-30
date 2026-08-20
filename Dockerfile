FROM golang:1.26 AS builder
ENV GOTOOLCHAIN=local
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /voltforge ./cmd/voltforge
RUN CGO_ENABLED=0 go build -o /voltforgectl ./cmd/voltforgectl

FROM golang:1.26
COPY --from=builder /voltforge /voltforge
COPY --from=builder /voltforgectl /voltforgectl
EXPOSE 56058
ENTRYPOINT ["/voltforge"]
