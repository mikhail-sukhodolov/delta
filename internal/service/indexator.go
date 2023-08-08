package service

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/samber/lo"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"offer-read-service/internal/repository"
	"sync"
	"time"
)

type IndexingResult struct {
	NumIndexed int
	Elapsed    time.Duration
}

type Indexator interface {
	Index(ctx context.Context) (IndexingResult, error)
}

type indexator struct {
	offerEnricher   OfferEnricher
	lock            sync.Mutex
	offerClient     offer_service.OfferServiceClient
	offerRepository repository.OfferRepository
	perPage         int
}

func NewIndexator(offerClient offer_service.OfferServiceClient, repo repository.OfferRepository, perPage int, offerEnricher OfferEnricher) Indexator {
	return &indexator{
		lock:            sync.Mutex{},
		offerClient:     offerClient,
		offerRepository: repo,
		perPage:         perPage,
		offerEnricher:   offerEnricher,
	}
}

func (s *indexator) Index(ctx context.Context) (IndexingResult, error) {
	logger := ctxzap.Extract(ctx)
	if !s.lock.TryLock() {
		return IndexingResult{}, fmt.Errorf("indexing is already started")
	}
	defer s.lock.Unlock()
	started := time.Now()
	numIndexed := 0
	for page := 1; ; page++ {
		offers, err := s.offerClient.SearchOffers(ctx, &offer_service.SearchOffersRequest{
			Pagination: &offer_service.Pagination{
				Limit:  lo.ToPtr(int32(s.perPage)),
				Offset: lo.ToPtr(int32((page - 1) * s.perPage)),
			},
			Sort: &offer_service.Sort{
				Field:     offer_service.SortField_ID,
				Direction: offer_service.SortDirection_DESC,
			},
			PriceFilter: offer_service.OfferPriceFilter_OFFER_PRICE_FILTER_WITH_EMPTY_PRICE,
		})
		if err != nil {
			return IndexingResult{}, fmt.Errorf("can't SearchOffers %w", err)
		}

		richOffers, err := s.offerEnricher.Enrich(ctx, offers.Offer)
		if err != nil {
			return IndexingResult{}, fmt.Errorf("can't enrich %w", err)
		}

		err = s.offerRepository.Update(ctx, richOffers)
		if err != nil {
			return IndexingResult{}, fmt.Errorf("can't update in elastic %w", err)
		}
		if page%10 == 0 || page == 1 {
			logger.Sugar().Infof("page=%d", page)
		}
		numIndexed += len(richOffers)
		if len(offers.Offer) < s.perPage {
			break
		}
	}

	return IndexingResult{NumIndexed: numIndexed, Elapsed: time.Since(started)}, nil
}
