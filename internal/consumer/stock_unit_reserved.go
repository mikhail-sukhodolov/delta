package consumer

import (
	"context"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/stock"
	"gitlab.int.tsum.com/preowned/simona/delta/core.git/retrying_consumer"
	"go.uber.org/zap"
	"offer-read-service/internal/repository"
	"offer-read-service/internal/service"
	"time"
)

func StockUnitReserved(offerClient offer_service.OfferServiceClient, offerEnricher service.OfferEnricher, offerRepository repository.OfferRepository) retrying_consumer.Handler[stock.StockUnitReservedEvent] {
	return func(ctx context.Context, event stock.StockUnitReservedEvent, _ retrying_consumer.Meta) error {
		time.Sleep(time.Second * 5)
		searchOffers, err := offerClient.SearchOffers(ctx, &offer_service.SearchOffersRequest{OfferCodes: []string{event.OfferCode}})
		if err != nil {
			return fmt.Errorf("offerClient.SearchOffers %w", err)
		}
		if len(searchOffers.Offer) == 0 {
			ctxzap.Info(ctx, "offers not found")
			return nil
		}
		offers, err := offerEnricher.Enrich(ctx, searchOffers.Offer)
		if err != nil {
			return fmt.Errorf("offerEnricher.Enrich %w", err)
		}
		err = offerRepository.Update(ctx, offers)
		if err != nil {
			return fmt.Errorf("offerRepository.Update %w", err)
		}
		return nil
	}
}

func RetryingMessageHandler(under retrying_consumer.Handler[stock.StockUnitReservedEvent]) retrying_consumer.Handler[stock.StockUnitReservedEvent] {
	return func(ctx context.Context, event stock.StockUnitReservedEvent, meta retrying_consumer.Meta) error {
		logger := ctxzap.Extract(ctx)
		err := retry.Do(
			func() error {
				return under(ctx, event, meta)
			},
			retry.Context(ctx),
			retry.LastErrorOnly(true),
			retry.OnRetry(func(n uint, err error) {
				logger.Error("retry attempt", zap.Uint("attempt", n), zap.Error(err))
			}),
		)
		if err != nil {
			logger.Error("final retry fail", zap.Error(err), zap.Any("event", event))
			return nil
		}
		return nil
	}
}
