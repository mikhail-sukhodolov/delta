package model

import "time"

type OfferStatus string

const (
	OfferStatusNew              OfferStatus = `new`
	OfferStatusSales            OfferStatus = `sales`
	OfferStatusInOrder          OfferStatus = `in_order`
	OfferStatusSold             OfferStatus = `sold`
	OfferStatusReturnedToSeller OfferStatus = `returned-to-seller`
)

type Offer struct {
	ID                              int         `json:"offer.id"`
	Code                            string      `json:"offer.code"`
	SellerID                        int         `json:"offer.seller_id"`
	Status                          OfferStatus `json:"offer.status"`
	IsNewCalculateDate              time.Time   `json:"offer.is_new_calculate_date"`
	IsSalesCalculateDate            time.Time   `json:"offer.is_sales_calculate_date"`
	IsOrderCalculateDate            time.Time   `json:"offer.is_order_calculate_date"`
	IsSoldCalculateDate             time.Time   `json:"offer.is_sold_calculate_date"`
	IsReturnedToSellerCalculateDate time.Time   `json:"offer.is_returned_to_seller_calculate_date"`
	Indexed                         time.Time   `json:"offer.indexed"`
}
