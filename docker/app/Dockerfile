FROM repo.int.tsum.com/tsum/core/golang:1.18 as builder
#ensure private repo is accessible
RUN mkdir -p -m 0600 ~/.ssh && ssh-keyscan gitlab.int.tsum.com >> ~/.ssh/known_hosts
RUN git config --global url.'git@gitlab.int.tsum.com:'.insteadOf 'https://gitlab.int.tsum.com/'
#download packages
WORKDIR /app
COPY ./go.mod ./go.sum .
RUN --mount=type=ssh --mount=type=cache,id=go_cache,target=/go/pkg/mod go mod download
#build server
COPY . .
RUN --mount=type=cache,id=go_cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache CGO_ENABLED=0 GOOS=linux go build -v ./cmd/offer-read/main.go

FROM alpine:3.15.4

COPY --from=builder /app/main main

CMD ["./main"]
