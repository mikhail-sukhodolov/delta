package model

import "time"

type OfferStatus string

const (
	OfferStatusPublished OfferStatus = `published`
)

type Offer struct {
	ID                       int         `json:"offer.id"`
	Code                     string      `json:"offer.code"`
	SellerID                 int         `json:"offer.seller_id"`
	Status                   OfferStatus `json:"offer.status"`
	IsPreparingCalculateDate time.Time   `json:"offer.is_preparing_calculate_date"`
	IsPublishedCalculateDate time.Time   `json:"offer.is_published_calculate_date"`
	IsReservingCalculateDate time.Time   `json:"offer.is_reserving_calculate_date"`
	IsSalesCalculateDate     time.Time   `json:"offer.is_sales_calculate_date"`
	IsReturnedCalculateDate  time.Time   `json:"offer.is_returned_calculate_date"`
}
