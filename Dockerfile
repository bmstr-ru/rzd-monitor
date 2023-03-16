FROM alpine:3.6
RUN apk update && apk add --no-cache ca-certificates tzdata && update-ca-certificates
COPY rzd-monitor /rzd-monitor
CMD ["/rzd-monitor"]