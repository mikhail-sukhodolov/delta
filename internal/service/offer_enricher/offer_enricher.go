package offer_enricher

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/samber/lo"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_read_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_write"
	v1 "gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/common/search_kit/v1"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/stock_service"
	"go.uber.org/zap"
	"offer-read-service/internal/model"
	"offer-read-service/internal/repository"
	"strings"
	"time"
)

const (
	stockReasonReleased = "released"
	stockReasonSold     = "sold"
	stockReasonReturned = "returned-to-seller"
	stockReasonLost     = "lost"
	stockReasonMoved    = "moved"
)

type enricher struct {
	catalogReadClient  catalog_read_service.CatalogReadSearchServiceClient
	catalogWriteClient catalog_write.CatalogWriteServiceClient
	stockClient        stock_service.StockServiceClient
	offerRepository    repository.OfferRepository
}

func NewEnricher(catalogReadClient catalog_read_service.CatalogReadSearchServiceClient, catalogWriteClient catalog_write.CatalogWriteServiceClient, stockClient stock_service.StockServiceClient, offerRepository repository.OfferRepository) *enricher {
	return &enricher{catalogReadClient: catalogReadClient, catalogWriteClient: catalogWriteClient, stockClient: stockClient, offerRepository: offerRepository}
}

func (s enricher) Enrich(ctx context.Context, offers []*offer_service.Offer) ([]model.Offer, error) {
	if len(offers) == 0 {
		return nil, nil
	}
	logger := ctxzap.Extract(ctx)
	itemCodes := lo.Map(offers, func(o *offer_service.Offer, index int) string {
		return o.ItemCode
	})
	offerCodes := lo.Map(offers, func(o *offer_service.Offer, _ int) string {
		return o.OfferCode
	})

	units, err := s.stockClient.ListStockUnits(ctx, &stock_service.ListStockUnitsRequest{
		Limit:      int32(len(offerCodes) + 1),
		OfferCodes: offerCodes,
	})
	if err != nil {
		return nil, fmt.Errorf("s.stockClient.ListStockUnits: %w", err)
	}

	offersFromDBSlice, err := s.offerRepository.ListOffer(ctx, v1.GetListRequest{
		Filters: &v1.GetListRequest_FilterGroup{
			Filters: []*v1.GetListRequest_FilterGroup_FieldFilter{
				{
					Field: "offer.code",
					Filter: &v1.GetListRequest_FilterGroup_FieldFilter_FilterTextIn{
						FilterTextIn: &v1.GetListRequest_FilterGroup_FieldFilter_FilterTypeTextIn{
							Value: offerCodes,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("s.repo.ListOffer: %w", err)
	}
	offersFromDB := lo.SliceToMap(offersFromDBSlice.Data, func(item model.Offer) (string, model.Offer) {
		return item.Code, item
	})

	wItem, err := s.catalogWriteClient.GetItemListByCodes(ctx, &catalog_write.GetItemListByCodesRequest{
		Codes: itemCodes,
	})
	if err != nil {
		return nil, fmt.Errorf("s.catalogWriteClient.GetItemListByCodes: %w", err)
	}
	catalogWriteItems := lo.SliceToMap(wItem.Data, func(item *catalog_write.ItemComposite) (string, *catalog_write.ItemComposite) {
		return item.Item.Code, item
	})

	offers = lo.Filter(offers, func(offer *offer_service.Offer, _ int) bool {
		ok := catalogWriteItems[offer.ItemCode] != nil
		if !ok {
			logger.Error("catalog write doesn't have offer", zap.String("item_code", offer.ItemCode))
		}
		return ok
	})
	if len(offers) == 0 {
		return nil, nil
	}

	return lo.Map(offers, func(offer *offer_service.Offer, _ int) model.Offer {
		offerFromDB := offersFromDB[offer.OfferCode]

		res := model.Offer{
			ID:                              int(offer.Id),
			Code:                            offer.OfferCode,
			SellerID:                        int(offer.SellerId),
			IsNewCalculateDate:              offerFromDB.IsNewCalculateDate,
			IsSalesCalculateDate:            offerFromDB.IsSalesCalculateDate,
			IsOrderCalculateDate:            offerFromDB.IsOrderCalculateDate,
			IsSoldCalculateDate:             offerFromDB.IsSoldCalculateDate,
			IsReturnedToSellerCalculateDate: offerFromDB.IsReturnedToSellerCalculateDate,
			Indexed:                         time.Now(),
		}

		var date time.Time
		res.Status, date = s.calculateStatus(offer, catalogWriteItems, units.StockUnits)

		switch {
		case res.Status == model.OfferStatusCodeNew:
			res.IsNewCalculateDate = date
		case res.Status == model.OfferStatusCodeSales:
			res.IsSalesCalculateDate = date
		case res.Status == model.OfferStatusCodeInOrder:
			res.IsOrderCalculateDate = date
		case res.Status == model.OfferStatusCodeSold:
			res.IsSoldCalculateDate = date
		case res.Status == model.OfferStatusCodeReturnedToSeller:
			res.IsReturnedToSellerCalculateDate = date
		}

		return res
	}), nil
}

func (s enricher) calculateStatus(
	offer *offer_service.Offer,
	catalogWriteOffers map[string]*catalog_write.ItemComposite,
	units []*stock_service.StockUnit,
) (model.OfferStatusCode, time.Time) {
	units = lo.Filter(units, func(item *stock_service.StockUnit, _ int) bool {
		return !strings.Contains(item.VersionClosingReason, "duplicate")
	})

	for _, unit := range units {
		if unit.OfferCode != offer.OfferCode {
			continue
		}

		if unit.IsAvailableForPurchase {
			item, ok := catalogWriteOffers[offer.ItemCode]
			if ok {
				switch {
				case item.Item.IsDraft:
					return model.OfferStatusCodeNew, catalogWriteOffers[offer.ItemCode].Item.CreatedAt.AsTime()
				case !lo.Contains(item.Item.PublicationFlags, catalog_write.ItemPublicationFlag_ITEM_PUBLICATION_FLAG_VISIBLE_IOS):
					return model.OfferStatusCodeNew, catalogWriteOffers[offer.ItemCode].Item.CreatedAt.AsTime()
				}
			}

			if offer.Price == nil || (offer.Price.CurrencyCode == "RUB" && offer.Price.Units < 1000) {
				return model.OfferStatusCodeNew, catalogWriteOffers[offer.ItemCode].Item.CreatedAt.AsTime()
			} else {
				return model.OfferStatusCodeSales, catalogWriteOffers[offer.ItemCode].Item.CreatedAt.AsTime()
			}
		}

		if unit.VersionClosingReason == stockReasonSold {
			return model.OfferStatusCodeSold, unit.VersionClosedAt.AsTime()
		}

		if unit.IsReserved {
			return model.OfferStatusCodeInOrder, unit.ReservedAt.AsTime()
		}

		if unit.VersionClosingReason == stockReasonReleased {
			return model.OfferStatusCodeInOrder, unit.VersionClosedAt.AsTime()
		}

		if unit.VersionClosingReason == stockReasonReturned {
			return model.OfferStatusCodeReturnedToSeller, unit.VersionClosedAt.AsTime()
		}

		if unit.VersionClosingReason == stockReasonLost {
			return model.OfferStatusCodeSales, catalogWriteOffers[offer.ItemCode].Item.CreatedAt.AsTime()
		}

		if unit.VersionClosingReason == stockReasonMoved {
			return model.OfferStatusCodeNew, catalogWriteOffers[offer.ItemCode].Item.CreatedAt.AsTime()
		}

	}

	return model.OfferStatusCodeNew, catalogWriteOffers[offer.ItemCode].Item.CreatedAt.AsTime()
}
