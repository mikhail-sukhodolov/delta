package service

import (
	"context"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"offer-read-service/internal/model"
)

type OfferEnricher interface {
	Enrich(ctx context.Context, offers []*offer_service.Offer) ([]model.Offer, error)
}
