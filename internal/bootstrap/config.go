package bootstrap

import (
	"time"

	"github.com/joho/godotenv"

	_ "github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ReleaseID        string
	Env              string `envconfig:"ENV" default:"development"`
	LogLevel         string `envconfig:"LOG_LEVEL" default:"info"`
	GRPC             GRPCServerConfig
	GrpcClientConfig GRPCClientConfig
	HTTP             HTTPServerConfig
	Sentry           SentryConfig
	ElasticAPM       ElasticAPM
	Elastic          ElasticConfig `envconfig:"ELASTIC"`
}

type GRPCServerConfig struct {
	ListenAddr               string        `envconfig:"GRPC_LISTEN_ADDR" default:":9090" required:"true"`
	KeepaliveTime            time.Duration `envconfig:"GRPC_KEEPALIVE_TIME" default:"30s" required:"true"`
	KeepaliveTimeout         time.Duration `envconfig:"GRPC_KEEPALIVE_TIMEOUT" default:"10s" required:"true"`
	RegisterReflectionServer bool          `envconfig:"GRPC_REGISTER_REFLECTION_SERVER" default:"true" required:"true"`
}

type GRPCClientConfig struct {
	OfferEndpoint       string `envconfig:"GRPC_OFFER_SERVICE_ADDR" required:"true"`
	CatalogReadEndpoint string `envconfig:"GRPC_CATALOG_READ_SERVICE_ADDR" required:"true"`
	StockEndpoint       string `envconfig:"GRPC_STOCK_SERVICE_ADDR" required:"true"`
}

type HTTPServerConfig struct {
	ListenAddr       string        `envconfig:"HTTP_LISTEN_ADDR" default:":8080"`
	KeepaliveTime    time.Duration `envconfig:"HTTP_KEEPALIVE_TIME" default:"30s"`
	KeepaliveTimeout time.Duration `envconfig:"HTTP_KEEPALIVE_TIMEOUT" default:"10s"`
}

type SentryConfig struct {
	DSN string `envconfig:"SENTRY_DSN"`
}

type ElasticAPM struct {
	ServiceName string `envconfig:"ELASTIC_APM_SERVICE_NAME" required:"true"`
	ServerURL   string `envconfig:"ELASTIC_APM_SERVER_URL" required:"true"`
	Environment string `envconfig:"ELASTIC_APM_ENVIRONMENT" required:"true"`
}

type ElasticConfig struct {
	Addresses      []string `envconfig:"ADDRESSES" required:"true"`
	OfferIndexName string   `envconfig:"OFFER_INDEX_NAME" default:"delta.offer_index" required:"true"`
	IndexPerPage   int      `envconfig:"INDEX_PER_PAGE" default:"100" required:"true"`
}

func NewConfig() (*Config, error) {
	config := Config{}
	_ = godotenv.Load()
	err := envconfig.Process("", &config)
	return &config, err
}