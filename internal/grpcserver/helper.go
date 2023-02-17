package grpcserver

import v1 "gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/common/search_kit/v1"

func buildGRPCPagination(pagination *v1.GetListRequest_Pagination, total int64) *v1.PaginationInfo {
	if pagination == nil {
		return nil
	}
	pageCount := total / int64(pagination.PerPage)
	if total%int64(pagination.PerPage) > 0 {
		pageCount++
	}
	page := pagination.Page
	if pageCount < page {
		page = pageCount
	}

	return &v1.PaginationInfo{
		Page:      uint32(page),
		PerPage:   pagination.PerPage,
		Total:     total,
		PageCount: pageCount,
	}
}

func buildGRPCSortInfo(sort *v1.GetListRequest_Sort) *v1.SortInfo {
	if sort == nil {
		return nil
	}
	return &v1.SortInfo{Field: sort.Field, Direction: sort.Direction}
}
