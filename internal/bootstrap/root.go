package bootstrap

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/samber/lo"
	"gitlab.int.tsum.com/core/libraries/corekit.git/kafka/broker"
	"gitlab.int.tsum.com/core/libraries/corekit.git/observability/logging"
	"gitlab.int.tsum.com/core/libraries/corekit.git/observability/tracing"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_read_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_write"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/stock"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/stock_service"
	grpc_helper "gitlab.int.tsum.com/preowned/simona/delta/core.git/grpc"
	"gitlab.int.tsum.com/preowned/simona/delta/core.git/retrying_consumer"
	"go.elastic.co/apm/module/apmgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"net/http"
	"offer-read-service/internal/consumer"
	"offer-read-service/internal/repository"
	"offer-read-service/internal/service"
	"offer-read-service/internal/service/offer_enricher"
	"sync"
)

// Объявляем константу для имени сервиса.
const (
	serviceName = "offer-read"
)

// Определение структуры Root, которая будет являться основным компонентом приложения.
type Root struct {
	// Конфигурация приложения
	Config *Config

	// GRPC сервер
	Server *grpc.Server

	// Определение инфраструктуры, включая HTTP сервер, Kafka потребителя и клиента Elasticsearch
	Infrastructure struct {
		HTTP          *http.Server
		KafkaConsumer broker.Consumer
		Elasticsearch *elasticsearch.Client
	}

	// Репозитории для работы с данными
	Repositories struct {
		OfferRepository       repository.OfferRepository
		OfferStatusRepository repository.OfferStatusRepository
	}

	// Клиенты для взаимодействия с внешними сервисами
	Clients struct {
		OfferClient        offer_service.OfferServiceClient
		CatalogReadClient  catalog_read_service.CatalogReadSearchServiceClient
		CatalogWriteClient catalog_write.CatalogWriteServiceClient
		StockClient        stock_service.StockServiceClient
	}

	// Сервисы, предоставляющие бизнес-логику
	Services struct {
		Indexator     service.Indexator
		OfferEnricher service.OfferEnricher
	}

	// Логгер для записи логов
	Logger *zap.Logger

	// Список фоновых задач
	backgroundJobs []func() error

	// Обработчики для остановки сервиса
	stopHandlers []func()

	// Трассировщик для мониторинга
	Tracer *apm.Tracer
}

// Регистрация фоновой задачи
func (r *Root) RegisterBackgroundJob(backgroundJob func() error) {
	r.backgroundJobs = append(r.backgroundJobs, backgroundJob)
}

// Регистрация обработчика остановки
func (r *Root) RegisterStopHandler(stopHandler func()) {
	r.stopHandlers = append(r.stopHandlers, stopHandler)
}

// Тип для опций инициализации Root
type Option func(app *Root)

// Функция создания нового Root с заданными параметрами и опциями
func NewRoot(ctx context.Context, config *Config, options ...Option) (*Root, error) {
	// Создание экземпляра Root с конфигурацией
	root := Root{Config: config}

	// Инициализация логгера
	root.Logger = lo.Must(logging.NewLogger(config.LogLevel, serviceName, config.ReleaseID)).
		WithOptions(zap.AddStacktrace(zap.PanicLevel))
	ctx = ctxzap.ToContext(ctx, root.Logger)

	// Инициализация трассировщика
	root.Tracer = lo.Must(tracing.NewAPMTracerFromConfig(tracing.Config{
		ServiceName: config.ElasticAPM.ServiceName,
		ServerURL:   config.ElasticAPM.ServerURL,
		Environment: config.ElasticAPM.Environment,
	}, config.ReleaseID))

	// Инициализация компонентов приложения
	root.initGRPCServer()
	root.initInfrastructure()
	root.initClients()
	root.initRepositories()
	root.initServices()
	root.initConsumers(ctx)
	root.initHTTPServer()
	lo.Must0(root.initSentry())

	// Применение дополнительных опций
	for _, option := range options {
		option(&root)
	}

	return &root, nil
}

// Запуск приложения
func (r *Root) Run(ctx context.Context) error {
	r.Logger.Info("starting application")
	defer r.stop() // Остановка при завершении

	// Канал для обработки ошибок фоновых задач
	errorsCh := make(chan error)
	for _, job := range r.backgroundJobs {
		_job := job
		go func() {
			errorsCh <- _job() // Запуск задачи и отправка результата в канал
		}()
	}

	// Ожидание завершения контекста или ошибки из канала
	select {
	case <-ctx.Done():
		return nil
	case err := <-errorsCh:
		return err
	}
}

// Функция остановки приложения
func (r *Root) stop() {
	// Создаем переменную 'wg' типа WaitGroup из пакета 'sync'.
	// WaitGroup используется для ожидания завершения нескольких горутин.
	var wg sync.WaitGroup

	// Добавляем в WaitGroup количество элементов, равное длине слайса 'stopHandlers'.
	// Это означает, что мы будем ожидать завершения всех обработчиков остановки.
	wg.Add(len(r.stopHandlers))

	// Проходим по всем обработчикам остановки в слайсе 'stopHandlers'.
	for _, handler := range r.stopHandlers {
		// Копируем обработчик в локальную переменную '_handler', чтобы избежать проблем с замыканием в горутине.
		_handler := handler

		// Запускаем горутину, анонимную функцию, для параллельного выполнения обработчика.
		go func() {
			// 'defer wg.Done()' будет вызван в конце выполнения горутины,
			// уменьшая счетчик WaitGroup на единицу.
			defer wg.Done()

			// Вызываем обработчик остановки '_handler'.
			_handler()
		}()
	}

	// Ожидаем завершения всех горутин, участвующих в WaitGroup.
	// Это блокирующий вызов, который дождется, пока счетчик WaitGroup не станет равным 0.
	wg.Wait()
}

// Инициализация GRPC сервера с определёнными настройками
func (r *Root) initGRPCServer() {
	// Настройки сервера
	options := []grpc_helper.ServerOption{
		grpc_helper.WithKeepaliveInterval(r.Config.GRPC.KeepaliveTime),
		grpc_helper.WithKeepaliveTimeout(r.Config.GRPC.KeepaliveTimeout),
		grpc_helper.WithLogger(r.Logger),
		grpc_helper.WithReportCodes([]codes.Code{codes.Unknown, codes.DeadlineExceeded, codes.Internal, codes.Unimplemented}),
		grpc_helper.WithTracingConfig(tracing.Config{
			ServiceName: r.Config.ElasticAPM.ServiceName,
			ServerURL:   r.Config.ElasticAPM.ServerURL,
			Environment: r.Config.ElasticAPM.Environment,
		}),
		grpc_helper.WithReleaseID(r.Config.ReleaseID),
	}

	// Регистрация сервера для отображения в GRPC
	if r.Config.GRPC.RegisterReflectionServer {
		options = append(options, grpc_helper.WithRegisterReflectionServer())
	}

	var err error
	r.Server, err = grpc_helper.NewServer(
		options...,
	)
	if err != nil {
		panic(err)
	}

	// Регистрация фоновой задачи для запуска GRPC сервера
	r.RegisterBackgroundJob(func() error {
		listener, err := net.Listen("tcp", r.Config.GRPC.ListenAddr)
		if err != nil {
			return err
		}
		r.Logger.Info(fmt.Sprintf("GRPC server started, listening on address: %s", r.Config.GRPC.ListenAddr))
		return r.Server.Serve(listener)
	})
	r.RegisterStopHandler(r.Server.GracefulStop)
}

func (r *Root) initClients() {
	conn, err := dial(r.Config.GrpcClientConfig.OfferEndpoint)
	if err != nil {
		panic(err)
	}
	r.Clients.OfferClient = offer_service.NewOfferServiceClient(conn)

	conn, err = dial(r.Config.GrpcClientConfig.CatalogReadEndpoint)
	if err != nil {
		panic(err)
	}
	r.Clients.CatalogReadClient = catalog_read_service.NewCatalogReadSearchServiceClient(conn)

	conn, err = dial(r.Config.GrpcClientConfig.CatalogWriteEndpoint)
	if err != nil {
		panic(err)
	}
	r.Clients.CatalogWriteClient = catalog_write.NewCatalogWriteServiceClient(conn)

	conn, err = dial(r.Config.GrpcClientConfig.StockEndpoint)
	if err != nil {
		panic(err)
	}
	r.Clients.StockClient = stock_service.NewStockServiceClient(conn)

}

func dial(target string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(apmgrpc.NewUnaryClientInterceptor()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc.Dial to '%s' service: '%w'", target, err)
	}
	return conn, nil
}

func (r *Root) initInfrastructure() {
	r.Infrastructure.KafkaConsumer = broker.NewConsumer(r.Config.Kafka.Config, r.Logger)
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: r.Config.Elastic.Addresses,
	})
	if err != nil {
		panic(err)
	}
	r.Infrastructure.Elasticsearch = client
}

func (r *Root) initRepositories() {
	repo, err := repository.NewElasticRepo(r.Infrastructure.Elasticsearch, r.Config.Elastic.OfferIndexName)
	if err != nil {
		panic(err)
	}
	r.Repositories.OfferRepository = repo

	offerStatusRepository, _ := repository.NewOfferStatusRepository()
	r.Repositories.OfferStatusRepository = offerStatusRepository
}

func (r *Root) initServices() {
	r.Services.OfferEnricher = offer_enricher.NewEnricher(
		r.Clients.CatalogReadClient,
		r.Clients.CatalogWriteClient,
		r.Clients.StockClient,
		r.Repositories.OfferRepository,
	)
	r.Services.Indexator = service.NewIndexator(
		r.Clients.OfferClient,
		r.Repositories.OfferRepository,
		r.Config.IndexatorConfig.IndexPerPage,
		r.Services.OfferEnricher,
	)
}

func (r *Root) initSentry() error {
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              r.Config.Sentry.DSN,
		Environment:      r.Config.Env,
		Release:          fmt.Sprintf("%s@%s", serviceName, r.Config.ReleaseID),
		AttachStacktrace: true,
	})
	if err != nil {
		return fmt.Errorf("unable to init sentry client: %w", err)
	}

	return nil
}

func (r *Root) initConsumers(ctx context.Context) {
	r.RegisterBackgroundJob(func() error {
		retrying_consumer.NewConsumer[stock.StockUnitReservedEvent](
			r.Config.Kafka.ConsumerGroupId,
			r.Config.Kafka.StockUnitReservedTopic,
			r.Infrastructure.KafkaConsumer,
			consumer.RetryingMessageHandler(consumer.StockUnitReserved(r.Clients.OfferClient, r.Services.OfferEnricher, r.Repositories.OfferRepository)),
			r.Tracer,
			r.Logger,
		).Run(ctx)
		return nil
	})
}
