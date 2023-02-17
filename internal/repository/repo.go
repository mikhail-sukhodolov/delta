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

type OfferStatusRepository interface {
	ListOfferStatus(context.Context) ([]model.OfferStatus, error)
}

type OfferStatusTempRepository struct{}

func (r *OfferStatusTempRepository) ListOfferStatus(context.Context) ([]model.OfferStatus, error) {
	return []model.OfferStatus{
		{
			Code:  model.OfferStatusCodeNew,
			Title: "Создано",
		},
		{
			Code:  model.OfferStatusCodeSales,
			Title: "Опубликовано",
		},
		{
			Code:  model.OfferStatusCodeInOrder,
			Title: "Заказано",
		},
		{
			Code:  model.OfferStatusCodeSold,
			Title: "Продано",
		},
		{
			Code:  model.OfferStatusCodeReturnedToSeller,
			Title: "Снят с продажи",
		},
	}, nil
}

func NewOfferStatusRepository() (OfferStatusRepository, error) {
	return &OfferStatusTempRepository{}, nil
}
