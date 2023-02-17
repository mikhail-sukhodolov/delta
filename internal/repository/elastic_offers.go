package repository

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	v1 "gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/common/search_kit/v1"
	"gitlab.int.tsum.com/preowned/simona/delta/core.git/abstractquery/builder"
	"go.uber.org/zap"
	"io"
	"offer-read-service/internal/model"
	"strings"
)

//go:embed index_body.json
var indexBody string

type elasticOfferRepo struct {
	client    *elasticsearch.Client
	indexName string
}

type elasticResponse struct {
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		}
		Hits []struct {
			Source model.Offer `json:"_source,omitempty"`
		} `json:"hits"`
	} `json:"hits"`
}

func NewElasticRepo(client *elasticsearch.Client, indexName string) (OfferRepository, error) {
	err := createOrUpdateIndex(client, indexName)
	if err != nil {
		return nil, err
	}
	return &elasticOfferRepo{client: client, indexName: indexName}, nil
}

func createOrUpdateIndex(client *elasticsearch.Client, indexName string) error {
	getResponse, err := client.Indices.Get([]string{indexName})
	if err != nil {
		return fmt.Errorf("can't get elastic index %s, error: %w", indexName, err)
	}
	if getResponse.StatusCode == 404 {
		err := elasticErr(client.Indices.Create(indexName, client.Indices.Create.WithBody(strings.NewReader(indexBody))))
		if err != nil {
			return err
		}
	} else if getResponse.StatusCode == 200 {
		err := elasticErr(client.Indices.PutMapping([]string{indexName}, strings.NewReader(gjson.Get(indexBody, "mappings").Raw)))
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *elasticOfferRepo) Update(ctx context.Context, offers []model.Offer) error {
	reader, err := modelsToReader(offers)
	if err != nil {
		return err
	}
	return elasticErr(e.client.Bulk(
		reader,
		e.client.Bulk.WithIndex(e.indexName),
		e.client.Bulk.WithContext(ctx),
	))
}

func (e *elasticOfferRepo) ListOffer(ctx context.Context, request v1.GetListRequest) (*ListResponse[model.Offer], error) {
	logger := ctxzap.Extract(ctx)
	p := *request.Pagination
	request.Pagination = &p

	offset := int64(0)
	if request.Pagination != nil && request.Pagination.Page > 1 {
		offset = (request.Pagination.Page - 1) * int64(request.Pagination.PerPage)
		request.Pagination.Page = 1 // For resetting offset build, because elastic can't translate it.
	}

	abstractQuery, err := builder.BuildFromSearchKit(request, []string{"*"}, e.indexName)
	if err != nil {
		return nil, fmt.Errorf("builder.BuildFromSearchKit: %w", err)
	}
	selectStatement, err := abstractQuery.ToElasticSelect()
	if err != nil {
		return nil, fmt.Errorf("abstractQuery.ToElasticSelect: %w", err)
	}
	buf, err := json.Marshal(struct {
		Query string `json:"query"`
	}{
		Query: selectStatement,
	})
	if err != nil {
		return nil, fmt.Errorf("json.Marshal: %w", err)
	}
	logger.Debug("translating elastic", zap.String("query", selectStatement))
	responseResp, err := e.client.SQL.Translate(
		bytes.NewBuffer(buf),
		e.client.SQL.Translate.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("e.client.SQL.Translate: %w", err)
	}
	if responseResp.IsError() {
		return nil, fmt.Errorf("responseResp.IsError: %s", responseResp.String())
	}
	defer responseResp.Body.Close()
	searchResp, err := e.client.Search(
		e.client.Search.WithIndex(e.indexName),
		e.client.Search.WithBody(responseResp.Body),
		e.client.Search.WithContext(ctx),
		e.client.Search.WithSource("true"),
		e.client.Search.WithFrom(int(offset)),
	)
	if err != nil {
		return nil, fmt.Errorf("elastic searchResp, error: %w", err)
	}
	if searchResp.IsError() {
		return nil, fmt.Errorf("elastic searchResp, error %s", searchResp.String())
	}
	defer searchResp.Body.Close()

	resp := elasticResponse{}
	responseBytes, err := io.ReadAll(searchResp.Body)
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll, error: %w", err)
	}
	err = json.Unmarshal(responseBytes, &resp)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal %w", err)
	}
	return &ListResponse[model.Offer]{
		Total: resp.Hits.Total.Value,
		Data: lo.Map(resp.Hits.Hits, func(item struct {
			Source model.Offer `json:"_source,omitempty"`
		}, _ int) model.Offer {
			return item.Source
		}),
	}, nil
}

func modelsToReader(offers []model.Offer) (io.Reader, error) {
	buffer := bytes.NewBuffer(nil)
	for _, o := range offers {
		byt, err := json.Marshal(o)
		if err != nil {
			return nil, err
		}
		buffer.WriteString(fmt.Sprintf(`{ "update": {"_id": "%s"} }`, o.Code))
		buffer.WriteByte('\n')
		buffer.Write([]byte(`{ "doc_as_upsert":true, "doc": `))
		buffer.Write(byt)
		buffer.Write([]byte(` }`))
		buffer.WriteByte('\n')
	}
	return buffer, nil
}

func elasticErr(resp *esapi.Response, err error) error {
	if err != nil {
		return fmt.Errorf("elastic response, error: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("elastic response, response %s", resp.String())
	}
	resp.Body.Close()
	return nil
}
