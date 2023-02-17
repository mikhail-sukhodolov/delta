package grpcserver

import (
	"context"
	"fmt"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/samber/lo"
	v1 "gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/common/search_kit/v1"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_read_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"gitlab.int.tsum.com/preowned/simona/delta/core.git/custom_error"
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
	if listResponse.Total == 0 || len(listResponse.Data) == 0 {
		return nil, &custom_error.ErrorNotFound{}
	}

	offersMap := lo.SliceToMap(listResponse.Data, func(item model.Offer) (string, model.Offer) {
		return item.Code, item
	})

	searchOffersResponse, err := s.root.Clients.OfferClient.SearchOffers(ctx, &offer_service.SearchOffersRequest{
		OfferCodes: lo.Map(listResponse.Data, func(item model.Offer, index int) string {
			return item.Code
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("OfferClient.SearchOffers %w", err)
	}

	listOfferStatuses, _ := s.root.Repositories.OfferStatusRepository.ListOfferStatus(ctx)
	listOfferStatusesMap := lo.SliceToMap(listOfferStatuses, func(offerStatus model.OfferStatus) (model.OfferStatusCode, model.OfferStatus) {
		return offerStatus.Code, offerStatus
	})

	return &offer_read_service.ListOffersResponse{
		Meta: &v1.ResponseMeta{
			Sort:       buildGRPCSortInfo(request.Data.Sort),
			Pagination: buildGRPCPagination(request.Data.Pagination, listResponse.Total),
		},
		Offers: lo.Map(searchOffersResponse.Offer, func(item *offer_service.Offer, _ int) *offer_read_service.ListOffersResponse_Offer {
			return &offer_read_service.ListOffersResponse_Offer{
				Id:            item.Id,
				OfferCode:     item.OfferCode,
				Price:         item.Price,
				SellerId:      item.SellerId,
				ItemCode:      item.ItemCode,
				InvoiceNumber: item.InvoiceNumber,
				Reason:        item.Reason,
				CreatedAt:     item.CreatedAt,
				ClosedAt:      item.ClosedAt,
				TaxRate:       item.TaxRate,
				InvoiceDate:   item.InvoiceDate,
				Status: &offer_read_service.ListOffersResponse_Offer_Status{
					Title:         listOfferStatusesMap[offersMap[item.OfferCode].Status].Title,
					Code:          string(offersMap[item.OfferCode].Status),
					CalculateDate: timestamppb.New(offersMap[item.OfferCode].GetStatusDate()),
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
	if filters != nil && len(filters.Filters) == 1 && filters.Filters[0] != nil &&
		filters.Filters[0].Field == fieldOfferStatus && filters.Filters[0].GetFilterTextIn() != nil &&
		len(filters.Filters[0].GetFilterTextIn().Value) == 1 {
		return filters.Filters[0].GetFilterTextIn().Value[0]
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
