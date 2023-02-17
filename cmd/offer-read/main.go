package main

import (
	"context"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_read_service"
	"log"
	"offer-read-service/internal/grpcserver"
	"os/signal"
	"syscall"

	"offer-read-service/internal/bootstrap"
)

// releaseID is set during the build in CI/CD pipeline using ldflags (eg.: go build -ldflags="-X 'main.releaseID=<release id>'").
var releaseID string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	config, err := bootstrap.NewConfig()
	if err != nil {
		log.Panicf("can't create new config.go: %v", err)
	}
	config.ReleaseID = releaseID

	root, err := bootstrap.NewRoot(config)
	if err != nil {
		log.Panicf("application could not been initialized: %v", err)
	}

	offer_read_service.RegisterOfferReadServiceServer(root.Server, grpcserver.NewServer(root))

	if err = root.Run(ctx); err != nil {
		log.Panicf("application terminated abnormally: %s", err)
	}
}
