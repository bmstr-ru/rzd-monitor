FROM golang:alpine AS builder
RUN apk update --no-cache && apk add --no-cache gcc musl-dev
ENV CGO_ENABLED 1
ENV GOOS linux
WORKDIR /build
ADD go.mod .
ADD go.sum .
RUN go mod download
COPY . .
RUN GOOS=linux GOARCH=amd64 go build .

FROM alpine
RUN apk update --no-cache && apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /build/rzd-monitor /app/rzd-monitor
COPY --from=builder /build/db.sqlite /app/db.sqlite
CMD ["./rzd-monitor"]