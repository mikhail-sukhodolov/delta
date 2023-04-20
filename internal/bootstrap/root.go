package bootstrap

import (
	"context"
	"fmt"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/getsentry/sentry-go"
	"gitlab.int.tsum.com/core/libraries/corekit.git/observability/tracing"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_read_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/catalog_write"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_service"
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/stock_service"
	"go.elastic.co/apm/module/apmgrpc/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"net/http"
	"net/http/pprof"
	"offer-read-service/internal/repository"
	"offer-read-service/internal/service"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gitlab.int.tsum.com/core/libraries/corekit.git/healthcheck"
	"gitlab.int.tsum.com/core/libraries/corekit.git/observability/logging"
	grpc_helper "gitlab.int.tsum.com/preowned/simona/delta/core.git/grpc"
	"gitlab.int.tsum.com/preowned/simona/delta/core.git/grpc/interceptor"
	"go.uber.org/zap"
)

const (
	serviceName = "offer-read"
)

type Root struct {
	Config         *Config
	Server         *grpc.Server
	Infrastructure struct {
		HTTP          *http.Server
		Elasticsearch *elasticsearch.Client
	}
	Repositories struct {
		OfferRepository       repository.OfferRepository
		OfferStatusRepository repository.OfferStatusRepository
	}
	Clients struct {
		OfferClient        offer_service.OfferServiceClient
		CatalogReadClient  catalog_read_service.CatalogReadSearchServiceClient
		CatalogWriteClient catalog_write.CatalogWriteServiceClient
		StockClient        stock_service.StockServiceClient
	}
	Services struct {
		Indexator service.Indexator
	}
	Logger         *zap.Logger
	backgroundJobs []func() error
	stopHandlers   []func()
}

func (r *Root) RegisterBackgroundJob(backgroundJob func() error) {
	r.backgroundJobs = append(r.backgroundJobs, backgroundJob)
}

func (r *Root) RegisterStopHandler(stopHandler func()) {
	r.stopHandlers = append(r.stopHandlers, stopHandler)
}

type Option func(app *Root)

func NewRoot(ctx context.Context, config *Config, options ...Option) (*Root, error) {
	root := Root{Config: config}

	var err error

	// Init logger
	root.Logger, err = logging.NewLogger(config.LogLevel, serviceName, config.ReleaseID)
	if err != nil {
		return nil, err
	}
	root.Logger = root.Logger.WithOptions(zap.AddStacktrace(zap.PanicLevel))

	root.initGRPCServer()
	root.initInfrastructure()
	root.initHTTPServer(ctx)
	root.initClients()
	root.initRepositories()
	root.initServices()
	err = root.initSentry()
	if err != nil {
		root.Logger.Sugar().Errorf("error on init sentry: %s", err)
	}

	for _, option := range options {
		option(&root)
	}

	return &root, nil
}

func (r *Root) Run(ctx context.Context) error {
	r.Logger.Info("starting application")
	defer r.stop()

	errorsCh := make(chan error)
	for _, job := range r.backgroundJobs {
		_job := job
		go func() {
			errorsCh <- _job()
		}()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errorsCh:
		return err
	}
}

func (r *Root) stop() {
	var wg sync.WaitGroup
	wg.Add(len(r.stopHandlers))
	for _, handler := range r.stopHandlers {
		_handler := handler
		go func() {
			defer wg.Done()
			_handler()
		}()
	}
	wg.Wait()
}

func (r *Root) initHTTPServer(ctx context.Context) {
	mux := http.NewServeMux()

	// Init pprof endpoints
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	// Init prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Init health check endpoint
	mux.Handle("/health", healthcheck.Handler(
		healthcheck.WithReleaseID(r.Config.ReleaseID),
	))
	mux.Handle("/full_index", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		go func() {
			r.Logger.Info("indexator is starting")

			_, err := r.Services.Indexator.Index(ctx)
			if err != nil {
				r.Logger.Error("couldn't indexing", zap.Error(err))
			}

			r.Logger.Info("indexator finished")
		}()

		writer.WriteHeader(200)
	}))

	r.Infrastructure.HTTP = &http.Server{
		Handler:     mux,
		Addr:        r.Config.HTTP.ListenAddr,
		IdleTimeout: r.Config.HTTP.KeepaliveTime + r.Config.HTTP.KeepaliveTimeout,
	}

	r.RegisterBackgroundJob(func() error {
		return r.Infrastructure.HTTP.ListenAndServe()
	})
	r.RegisterStopHandler(func() { _ = r.Infrastructure.HTTP.Shutdown(context.Background()) })
}

var (
	sensitiveKeys []string
)

func (r *Root) initGRPCServer() {
	marshaller, err := interceptor.NewMarshaller(sensitiveKeys)
	if err != nil {
		panic(err)
	}

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
		grpc_helper.WithPayloadJsonPbMarshaller(marshaller),
	}

	if r.Config.GRPC.RegisterReflectionServer {
		options = append(options, grpc_helper.WithRegisterReflectionServer())
	}

	r.Server, err = grpc_helper.NewServer(
		options...,
	)
	if err != nil {
		panic(err)
	}

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
	r.Services.Indexator = service.NewIndexator(
		r.Clients.OfferClient,
		r.Clients.CatalogReadClient,
		r.Clients.CatalogWriteClient,
		r.Clients.StockClient,
		r.Repositories.OfferRepository,
		r.Logger,
		r.Config.Elastic.IndexPerPage,
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
