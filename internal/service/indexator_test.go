package service

import (
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_read_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_write"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/common/money"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/stock_service"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	"offer-read-service/internal/model"
	"offer-read-service/internal/repository"
	"reflect"
	"sync"
	"testing"
	"time"
)

func Test_indexator_calculateStatus(t *testing.T) {
	type fields struct {
		lock               sync.Mutex
		offerClient        offer_service.OfferServiceClient
		catalogReadClient  catalog_read_service.CatalogReadSearchServiceClient
		catalogWriteClient catalog_write.CatalogWriteServiceClient
		stockClient        stock_service.StockServiceClient
		repo               repository.OfferRepository
		logger             *zap.Logger
		perPage            int
	}

	testTime := time.Now().UTC()

	type args struct {
		offer              *offer_service.Offer
		catalogReadOffers  map[string]*catalog_read_service.ItemComposite
		catalogWriteOffers map[string]*catalog_write.ItemComposite
		units              []*stock_service.StockUnit
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   model.OfferStatusCode
		want1  time.Time
	}{
		{
			name: "stockReasonReleased",
			args: args{
				offer: &offer_service.Offer{
					Id:        1,
					OfferCode: "OFFER-CODE-1",
					Price: &money.Money{
						CurrencyCode: "RUB",
						Units:        17_000,
					},
					SellerId: 1,
					ItemCode: "ITEM-CODE-1",
				},
				catalogReadOffers: map[string]*catalog_read_service.ItemComposite{
					"ITEM-CODE-1": {
						Item: &catalog_read_service.Item{InStock: false},
					},
				},
				catalogWriteOffers: map[string]*catalog_write.ItemComposite{
					"ITEM-CODE-1": {
						Item: &catalog_write.Item{
							CreatedAt: timestamppb.New(testTime),
						},
					},
				},
				units: []*stock_service.StockUnit{
					{
						OfferCode:            "OFFER-CODE-1",
						VersionClosingReason: "released",
						VersionClosedAt:      timestamppb.New(testTime),
					},
				},
			},
			want:  model.OfferStatusCodeInOrder,
			want1: testTime,
		},
		{
			name: "nil_price",
			args: args{
				offer: &offer_service.Offer{
					Id:        1,
					OfferCode: "OFFER-CODE-2",
					Price:     nil,
					SellerId:  1,
					ItemCode:  "ITEM-CODE-2",
				},
				catalogReadOffers: map[string]*catalog_read_service.ItemComposite{
					"ITEM-CODE-2": {
						Item: &catalog_read_service.Item{InStock: false},
					},
				},
				catalogWriteOffers: map[string]*catalog_write.ItemComposite{
					"ITEM-CODE-2": {
						Item: &catalog_write.Item{
							CreatedAt: timestamppb.New(testTime),
						},
					},
				},
				units: []*stock_service.StockUnit{
					{
						OfferCode:            "OFFER-CODE-1",
						VersionClosingReason: "released",
						VersionClosedAt:      timestamppb.New(testTime),
					},
				},
			},
			want:  model.OfferStatusCodeNew,
			want1: testTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &indexator{
				lock:               tt.fields.lock,
				offerClient:        tt.fields.offerClient,
				catalogReadClient:  tt.fields.catalogReadClient,
				catalogWriteClient: tt.fields.catalogWriteClient,
				stockClient:        tt.fields.stockClient,
				repo:               tt.fields.repo,
				logger:             tt.fields.logger,
				perPage:            tt.fields.perPage,
			}
			got, got1 := s.calculateStatus(tt.args.offer, tt.args.catalogReadOffers, tt.args.catalogWriteOffers, tt.args.units)
			if got != tt.want {
				t.Errorf("calculateStatus() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("calculateStatus() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
