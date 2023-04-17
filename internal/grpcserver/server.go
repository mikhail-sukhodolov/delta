package grpcserver

import (
	"context"
	"fmt"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/samber/lo"
	v1 "gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/common/search_kit/v1"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_read_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"google.golang.org/protobuf/types/known/timestamppb"
	"offer-read-service/internal/bootstrap"
	"offer-read-service/internal/model"
)

const (
	fieldOfferStatus = "offer.status"
)

type server struct {
	root *bootstrap.Root
	offer_read_service.UnimplementedOfferReadServiceServer
}

func NewServer(root *bootstrap.Root) offer_read_service.OfferReadServiceServer {
	return &server{root: root}
}

func (s server) ListOffers(ctx context.Context, request *offer_read_service.ListOffersRequest) (*offer_read_service.ListOffersResponse, error) {
	err := validation.ValidateStruct(request,
		validation.Field(&request.Data, validation.Required),
	)
	if err != nil {
		return nil, err
	}

	err = validation.ValidateStruct(request.Data,
		validation.Field(&request.Data.Pagination, validation.Required),
	)
	if err != nil {
		return nil, err
	}
	err = validation.ValidateStruct(request.Data.Pagination,
		validation.Field(&request.Data.Pagination.PerPage, validation.Required),
		validation.Field(&request.Data.Pagination.PerPage, validation.Min(uint32(1))),
		validation.Field(&request.Data.Pagination.Page, validation.Required),
		validation.Field(&request.Data.Pagination.Page, validation.Min(int64(1))),
	)
	if err != nil {
		return nil, err
	}

	listResponse, err := s.root.Repositories.OfferRepository.ListOffer(ctx, buildListOffersRepoRequest(*request))
	if err != nil {
		return nil, fmt.Errorf("OfferRepository.ListOffer %w", err)
	}

	searchOffersResponse, err := s.root.Clients.OfferClient.SearchOffers(ctx, &offer_service.SearchOffersRequest{
		OfferCodes: lo.Map(listResponse.Data, func(item model.Offer, index int) string {
			return item.Code
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("OfferClient.SearchOffers %w", err)
	}
	offersFromOfferSVCMap := lo.SliceToMap(searchOffersResponse.Offer, func(item *offer_service.Offer) (string, *offer_service.Offer) {
		if item == nil {
			return "", nil
		}
		return item.OfferCode, item
	})

	listOfferStatuses, _ := s.root.Repositories.OfferStatusRepository.ListOfferStatus(ctx)
	listOfferStatusesMap := lo.SliceToMap(listOfferStatuses, func(offerStatus model.OfferStatus) (model.OfferStatusCode, model.OfferStatus) {
		return offerStatus.Code, offerStatus
	})

	listResponse.Data = lo.Filter(listResponse.Data, func(item model.Offer, _ int) bool {
		_, ok := offersFromOfferSVCMap[item.Code]
		return ok
	})
	return &offer_read_service.ListOffersResponse{
		Meta: &v1.ResponseMeta{
			Sort:       buildGRPCSortInfo(request.Data.Sort),
			Pagination: buildGRPCPagination(request.Data.Pagination, listResponse.Total),
		},
		Offers: lo.Map(listResponse.Data, func(item model.Offer, _ int) *offer_read_service.ListOffersResponse_Offer {
			return &offer_read_service.ListOffersResponse_Offer{
				Id:            int64(item.ID),
				OfferCode:     item.Code,
				Price:         offersFromOfferSVCMap[item.Code].Price,
				SellerId:      int64(item.SellerID),
				ItemCode:      offersFromOfferSVCMap[item.Code].ItemCode,
				InvoiceNumber: offersFromOfferSVCMap[item.Code].InvoiceNumber,
				Reason:        offersFromOfferSVCMap[item.Code].Reason,
				CreatedAt:     offersFromOfferSVCMap[item.Code].CreatedAt,
				ClosedAt:      offersFromOfferSVCMap[item.Code].ClosedAt,
				TaxRate:       offersFromOfferSVCMap[item.Code].TaxRate,
				InvoiceDate:   offersFromOfferSVCMap[item.Code].InvoiceDate,
				Status: &offer_read_service.ListOffersResponse_Offer_Status{
					Title:         listOfferStatusesMap[item.Status].Title,
					Code:          string(item.Status),
					CalculateDate: timestamppb.New(item.GetStatusDate()),
				},
			}
		}),
	}, nil
}

func buildListOffersRepoRequest(request offer_read_service.ListOffersRequest) v1.GetListRequest {
	listRequest := *request.Data

	offerStatusFilterValue := getOfferStatusFilterValue(request.Data.Filters)
	if offerStatusFilterValue != "" {
		listRequest.Sort = &v1.GetListRequest_Sort{
			Field:     fmt.Sprintf(`offer.is_%s_calculate_date`, offerStatusFilterValue),
			Direction: v1.SortDirection_SORT_DIRECTION_DESC,
		}
	}

	return listRequest
}

func getOfferStatusFilterValue(filters *v1.GetListRequest_FilterGroup) string {
	if filters == nil {
		return ""
	}

	for _, filter := range filters.Filters {
		if filter.Field == "offer.status" {
			switch {
			case lo.Contains(filter.GetFilterTextIn().Value, string(model.OfferStatusCodeNew)):
				return "new"
			case lo.Contains(filter.GetFilterTextIn().Value, string(model.OfferStatusCodeSales)):
				return "sales"
			case lo.Contains(filter.GetFilterTextIn().Value, string(model.OfferStatusCodeInOrder)):
				return "order"
			case lo.Contains(filter.GetFilterTextIn().Value, string(model.OfferStatusCodeSold)):
				return "sold"
			case lo.Contains(filter.GetFilterTextIn().Value, string(model.OfferStatusCodeReturnedToSeller)):
				return "returned_to_seller"
			}
		}
	}

	return ""
}

func (s server) ListOffersConfig(ctx context.Context, _ *offer_read_service.ListOffersConfigRequest) (*offer_read_service.ListOffersConfigResponse, error) {
	listOfferStatus, _ := s.root.Repositories.OfferStatusRepository.ListOfferStatus(ctx)

	return &offer_read_service.ListOffersConfigResponse{
		Data: &v1.GetListConfigResponse{
			Filters: []*v1.GetListConfigResponse_Filter{
				{
					Variant: &v1.GetListConfigResponse_Filter_Field_{
						Field: &v1.GetListConfigResponse_Filter_Field{
							Label:     "Статус",
							FieldName: fieldOfferStatus,
							Filters: []*v1.GetListConfigResponse_FieldFilter{
								{
									Type: v1.FilterType_FILTER_TYPE_TEXT_IN,
									Options: lo.Map(listOfferStatus, func(offerStatus model.OfferStatus, _ int) *v1.GetListConfigResponse_FieldFilter_FieldOption {
										return &v1.GetListConfigResponse_FieldFilter_FieldOption{
											Id:   string(offerStatus.Code),
											Text: offerStatus.Title,
										}
									}),
								},
							},
						},
					},
				},
			},
		},
	}, nil
}
