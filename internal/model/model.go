package model

import "time"

type Offer struct {
	ID       int
	Code     string
	SellerID int

	IsPreparing              bool
	IsPreparingCalculateDate time.Time

	IsPublished              bool
	IsPublishedCalculateDate time.Time

	IsReserving              bool
	IsReservingCalculateDate time.Time

	IsSales              bool
	IsSalesCalculateDate time.Time

	IsReturned              bool
	IsReturnedCalculateDate time.Time
}
