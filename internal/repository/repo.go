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

type OfferStatusTempRepository struct {
}

func (r *OfferStatusTempRepository) ListOfferStatus(context.Context) ([]model.OfferStatus, error) {
	return []model.OfferStatus{
		model.OfferStatus{
			Code:  model.OfferStatusCodeNew,
			Title: "Новый",
		},
		model.OfferStatus{
			Code:  model.OfferStatusCodeSales,
			Title: "В продаже",
		},
		model.OfferStatus{
			Code:  model.OfferStatusCodeInOrder,
			Title: "В заказе",
		},
		model.OfferStatus{
			Code:  model.OfferStatusCodeSold,
			Title: "Продан",
		},
		model.OfferStatus{
			Code:  model.OfferStatusCodeReturnedToSeller,
			Title: "Возвращён",
		},
	}, nil
}

func NewOfferStatusRepository() (OfferStatusRepository, error) {
	return &OfferStatusTempRepository{}, nil
}
