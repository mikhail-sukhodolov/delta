package bootstrap

import (
	"context"
	"fmt"
	"github.com/getsentry/sentry-go"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"gitlab.int.tsum.com/core/libraries/corekit.git/observability/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"net"
	"net/http"
	"net/http/pprof"
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
		HTTP *http.Server
	}
	Repositories   struct{}
	Services       struct{}
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

func NewRoot(config *Config, options ...Option) (*Root, error) {
	root := Root{Config: config}

	var err error

	// Init logger
	root.Logger, err = logging.NewLogger(config.LogLevel, serviceName, config.ReleaseID)
	if err != nil {
		return nil, err
	}

	root.initHTTPServer()
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

func (r *Root) initHTTPServer() {
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

	grpc_zap.JsonPbMarshaller = marshaller
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

func (r *Root) initRepositories() {
}

func (r *Root) initServices() {
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
