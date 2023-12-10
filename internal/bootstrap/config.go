package bootstrap

// Импорт используемых пакетов
import (
	// Импорт библиотеки для работы с Kafka
	"gitlab.int.tsum.com/core/libraries/corekit.git/kafka/broker"
	"time" // Пакет для работы с временем

	// Библиотека godotenv для загрузки переменных окружения из файла .env
	"github.com/joho/godotenv"

	// Псевдоним импорта, который не используется напрямую, но нужен для инициализации пакета
	_ "github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig" // Библиотека для связывания переменных окружения с полями структуры
)

// Определение структуры Config для хранения конфигурации приложения
type Config struct {
	ReleaseID        string           // ID релиза
	Env              string           `envconfig:"ENV" default:"development"` // Окружение, по умолчанию "development"
	LogLevel         string           `envconfig:"LOG_LEVEL" default:"info"`  // Уровень логирования, по умолчанию "info"
	GRPC             GRPCServerConfig // Конфигурация gRPC сервера
	GrpcClientConfig GRPCClientConfig // Конфигурация gRPC клиента
	HTTP             HTTPServerConfig // Конфигурация HTTP сервера
	Sentry           SentryConfig     // Конфигурация Sentry
	ElasticAPM       ElasticAPM       // Конфигурация Elastic APM
	Elastic          ElasticConfig    `envconfig:"ELASTIC"` // Конфигурация ElasticSearch
	IndexatorConfig  IndexatorConfig  // Конфигурация индексатора
	Kafka            KafkaConfig      // Конфигурация Kafka
}

// Определение структуры IndexatorConfig
type IndexatorConfig struct {
	IndexPerPage int `envconfig:"INDEX_PER_PAGE" default:"50" required:"true"` // Количество индексов на страницу
}

// Определение структуры GRPCServerConfig для конфигурации gRPC сервера
type GRPCServerConfig struct {
	ListenAddr               string        `envconfig:"GRPC_LISTEN_ADDR" default:":9090" required:"true"`               // Адрес прослушивания
	KeepaliveTime            time.Duration `envconfig:"GRPC_KEEPALIVE_TIME" default:"30s" required:"true"`              // Время keepalive
	KeepaliveTimeout         time.Duration `envconfig:"GRPC_KEEPALIVE_TIMEOUT" default:"10s" required:"true"`           // Таймаут keepalive
	RegisterReflectionServer bool          `envconfig:"GRPC_REGISTER_REFLECTION_SERVER" default:"true" required:"true"` // Регистрация сервера рефлексии
}

// Определение структуры GRPCClientConfig для конфигурации gRPC клиента
type GRPCClientConfig struct {
	OfferEndpoint        string `envconfig:"GRPC_OFFER_SERVICE_ADDR" required:"true"`         // Адрес сервиса предложений
	CatalogReadEndpoint  string `envconfig:"GRPC_CATALOG_READ_SERVICE_ADDR" required:"true"`  // Адрес сервиса чтения каталога
	StockEndpoint        string `envconfig:"GRPC_STOCK_SERVICE_ADDR" required:"true"`         // Адрес сервиса запасов
	CatalogWriteEndpoint string `envconfig:"GRPC_CATALOG_WRITE_SERVICE_ADDR" required:"true"` // Адрес сервиса записи каталога
}

// Определение структуры HTTPServerConfig для конфигурации HTTP сервера
type HTTPServerConfig struct {
	ListenAddr       string        `envconfig:"HTTP_LISTEN_ADDR" default:":8080"`     // Адрес прослушивания
	KeepaliveTime    time.Duration `envconfig:"HTTP_KEEPALIVE_TIME" default:"30s"`    // Время keepalive
	KeepaliveTimeout time.Duration `envconfig:"HTTP_KEEPALIVE_TIMEOUT" default:"10s"` // Таймаут keepalive
}

// Определение структуры SentryConfig для конфигурации Sentry
type SentryConfig struct {
	DSN string `envconfig:"SENTRY_DSN"` // DSN для Sentry
}

// Определение структуры ElasticAPM для конфигурации Elastic APM
type ElasticAPM struct {
	ServiceName string `envconfig:"ELASTIC_APM_SERVICE_NAME" required:"true"` // Название сервиса
	ServerURL   string `envconfig:"ELASTIC_APM_SERVER_URL" required:"true"`   // URL сервера
	Environment string `envconfig:"ELASTIC_APM_ENVIRONMENT" required:"true"`  // Окружение
}

// Определение структуры ElasticConfig для конфигурации ElasticSearch
type ElasticConfig struct {
	Addresses      []string `envconfig:"ADDRESSES" required:"true"`                                    // Адреса ElasticSearch
	OfferIndexName string   `envconfig:"OFFER_INDEX_NAME" default:"delta.offer_index" required:"true"` // Название индекса предложений
}

// Определение структуры KafkaConfig для конфигурации Kafka
type KafkaConfig struct {
	broker.Config          `envconfig:"KAFKA"` // Настройки брокера Kafka
	Enabled                bool                `envconfig:"KAFKA_ENABLED" default:"true"`              // Включение Kafka
	ConsumerGroupId        string              `envconfig:"CONSUMER_GROUP_ID" required:"true"`         // ID группы потребителей
	StockUnitReservedTopic string              `envconfig:"STOCK_UNIT_RESERVED_TOPIC" required:"true"` // Тема для зарезервированных единиц запаса
}

// Функция NewConfig создает и возвращает новую конфигурацию
func NewConfig() (*Config, error) {
	config := Config{}
	_ = godotenv.Load()                   // Загрузка переменных окружения из файла .env
	err := envconfig.Process("", &config) // Связывание переменных окружения с полями структуры Config
	return &config, err
}
