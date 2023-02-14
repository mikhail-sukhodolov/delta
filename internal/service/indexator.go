package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_read_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/stock_service"
	"go.uber.org/zap"
	"offer-read-service/internal/model"
	"offer-read-service/internal/repository"
	"sync"
	"time"
)

type IndexingResult struct {
	NumIndexed int
	Elapsed    time.Duration
}

type Indexator interface {
	Index(ctx context.Context) (*IndexingResult, error)
}

type indexator struct {
	lock              sync.Mutex
	offerClient       offer_service.OfferServiceClient
	catalogReadClient catalog_read_service.CatalogReadSearchServiceClient
	stockClient       stock_service.StockServiceClient
	repo              repository.OfferRepository
	logger            *zap.Logger
	perPage           int
}

func NewIndexator(
	offerClient offer_service.OfferServiceClient,
	catalogReadClient catalog_read_service.CatalogReadSearchServiceClient,
	stockClient stock_service.StockServiceClient,
	repo repository.OfferRepository,
	logger *zap.Logger,
	perPage int,
) Indexator {
	return &indexator{
		lock:              sync.Mutex{},
		offerClient:       offerClient,
		catalogReadClient: catalogReadClient,
		stockClient:       stockClient,
		repo:              repo,
		logger:            logger.Named("indexator"),
		perPage:           perPage,
	}
}

func (s *indexator) Index(ctx context.Context) (*IndexingResult, error) {
	logger := s.logger.With(zap.String("trace.id", uuid.New().String()))
	if !s.lock.TryLock() {
		return nil, fmt.Errorf("indexing is already started")
	}
	defer s.lock.Unlock()

	started := time.Now()
	numIndexed := 0
	for page := 1; ; page++ {
		offers, err := s.offerClient.SearchOffers(ctx, &offer_service.SearchOffersRequest{
			Sort: &offer_service.Sort{
				Field: offer_service.SortField_ID,
			},
			Pagination: &offer_service.Pagination{
				Limit:  lo.ToPtr(int32(s.perPage)),
				Offset: lo.ToPtr(int32((page - 1) * s.perPage)),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("can't ListUsers %w", err)
		}
		logger.Sugar().Infof("list offer %d", len(offers.Offer))
		richOffers, err := s.enrich(ctx, offers.Offer)
		if err != nil {
			return nil, fmt.Errorf("can't enrich %w", err)
		}
		logger.Sugar().Infof("enrich offers %d", len(richOffers))

		err = s.repo.Update(ctx, richOffers)
		if err != nil {
			return nil, fmt.Errorf("can't update in elastic %w", err)
		}

		numIndexed += len(offers.Offer)
		if len(offers.Offer) < s.perPage {
			break
		}
	}

	return &IndexingResult{NumIndexed: numIndexed, Elapsed: time.Since(started)}, nil
}

func (s *indexator) enrich(ctx context.Context, offers []*offer_service.Offer) ([]model.Offer, error) {
	itemCodes := lo.Map(offers, func(o *offer_service.Offer, index int) string {
		return o.ItemCode
	})
	offerCodes := lo.Map(offers, func(o *offer_service.Offer, _ int) string {
		return o.OfferCode
	})
	itemResp, err := s.catalogReadClient.GetItemsByCodes(ctx, &catalog_read_service.GetItemsByCodesRequest{
		Codes: itemCodes,
	})
	if err != nil {
		return nil, fmt.Errorf("s.catalogReadClient.GetItemsByCodes: %w", err)
	}
	_ = lo.SliceToMap(itemResp.Items, func(item *catalog_read_service.ItemComposite) (string, *catalog_read_service.ItemComposite) {
		return item.Item.Code, item
	})

	units, err := s.stockClient.ListStockUnits(ctx, &stock_service.ListStockUnitsRequest{
		Limit:      int32(len(offerCodes) + 1),
		OfferCodes: offerCodes,
	})
	if err != nil {
		return nil, fmt.Errorf("s.stockClient.ListStockUnits: %w", err)
	}

	return lo.Map(offers, func(offer *offer_service.Offer, _ int) model.Offer {
		res := model.Offer{
			ID:       int(offer.Id),
			Code:     offer.ItemCode,
			SellerID: int(offer.SellerId),
		}

		_, _ = lo.Find(units.StockUnits, func(s *stock_service.StockUnit) bool {
			return s.OfferCode == offer.OfferCode
		})

		res.Status = model.OfferStatusPublished

		return res
	}), nil
}

const (
	stockReasonReleased = "released"
	stockReasonReturned = "returned-to-seller"
)

func isPreparing(stockItem *stock_service.StockUnit, itemPublished *catalog_read_service.ItemComposite) (bool, time.Time) {
	return stockItem == nil || (itemPublished != nil && (!stockItem.IsAvailableForPurchase || !itemPublished.Item.InStock)), time.Now()
}

func isPublished(itemPublished *catalog_read_service.ItemComposite) (bool, time.Time) {
	return itemPublished != nil, time.Now()
}

func isSales(stockItem *stock_service.StockUnit) (bool, time.Time) {
	return stockItem != nil && stockItem.VersionClosingReason == stockReasonReleased, time.Now()
}

func isReturned(stockItem *stock_service.StockUnit) (bool, time.Time) {
	return stockItem != nil && stockItem.VersionClosingReason == stockReasonReturned, time.Now()
}
