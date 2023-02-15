package model

import "time"

type OfferStatusCode string

const (
	OfferStatusCodeNew              OfferStatusCode = `new`
	OfferStatusCodeSales            OfferStatusCode = `sales`
	OfferStatusCodeInOrder          OfferStatusCode = `in_order`
	OfferStatusCodeSold             OfferStatusCode = `sold`
	OfferStatusCodeReturnedToSeller OfferStatusCode = `returned-to-seller`
)

type OfferStatus struct {
	Code  OfferStatusCode
	Title string
}

type Offer struct {
	ID                              int             `json:"offer.id"`
	Code                            string          `json:"offer.code"`
	SellerID                        int             `json:"offer.seller_id"`
	Status                          OfferStatusCode `json:"offer.status"`
	IsNewCalculateDate              time.Time       `json:"offer.is_new_calculate_date"`
	IsSalesCalculateDate            time.Time       `json:"offer.is_sales_calculate_date"`
	IsOrderCalculateDate            time.Time       `json:"offer.is_order_calculate_date"`
	IsSoldCalculateDate             time.Time       `json:"offer.is_sold_calculate_date"`
	IsReturnedToSellerCalculateDate time.Time       `json:"offer.is_returned_to_seller_calculate_date"`
	Indexed                         time.Time       `json:"offer.indexed"`
}
