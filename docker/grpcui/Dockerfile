FROM golang:1.18.1-alpine3.15 AS builder

RUN apk update && apk add --no-cache git

RUN go install github.com/fullstorydev/grpcui/cmd/grpcui@v1.3.0

COPY start.sh /usr/local/scripts/
RUN chmod +x /usr/local/scripts/*.sh

FROM alpine:3.15.4

COPY --from=builder /usr/local/scripts /usr/local/scripts
COPY --from=builder /go/bin/grpcui /usr/bin/grpcui

RUN apk update && apk add --no-cache bash
RUN adduser -S -u 10001 user
USER user

CMD ["/usr/local/scripts/start.sh"]
