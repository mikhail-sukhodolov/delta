package repository

import (
	"context"
	v1 "gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/common/search_kit/v1"
	"offer-read-service/internal/model"
)

type ListResponse[T any] struct {
	Total int64
	Data  []T
}

type OfferRepository interface {
	Update(context.Context, []model.Offer) error
	ListOffer(context.Context, v1.GetListRequest) (*ListResponse[model.Offer], error)
}
