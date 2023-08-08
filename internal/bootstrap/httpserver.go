package bootstrap

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/lo"
	"gitlab.int.tsum.com/core/libraries/corekit.git/healthcheck"
	"gitlab.int.tsum.com/core/libraries/corekit.git/observability/tracing"
	"gitlab.int.tsum.com/preowned/simona/delta/core.git/log_key"
	"go.elastic.co/apm/module/apmhttp/v2"
	"go.elastic.co/apm/v2"
	"go.uber.org/zap"
	"net/http"
	"net/http/pprof"
	"offer-read-service/internal/service"
)

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
	mux.Handle("/full_index", r.defaultHTTPHandler(fullIndexHandler(r.Services.Indexator)))

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

func fullIndexHandler(indexator service.Indexator) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		go func() {
			ctx := apm.DetachedContext(request.Context())
			logger := ctxzap.Extract(ctx).Named("full_index")
			logger.Info("indexator is starting")
			result, err := indexator.Index(ctx)
			if err != nil {
				logger.Error("couldn't indexing", zap.Error(err))
				return
			}
			logger.Info("indexator finished", zap.Reflect("result", fmt.Sprintf("%+v", result)))
		}()

		writer.WriteHeader(200)
	})
}

func loggingHandler(h http.Handler, logger *zap.Logger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		transaction := apm.TransactionFromContext(ctx)
		ctx = ctxzap.ToContext(ctx, logger.With(zap.String("trace.id", transaction.TraceContext().Trace.String())))
		request = apmhttp.RequestWithContext(ctx, request)
		h.ServeHTTP(writer, request)
	}
}
func recoveryHandler(h http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				ctxzap.Error(request.Context(), "panic", zap.Any(log_key.PanicReason, r), zap.Stack("stacktrace"))
			}
		}()
		h.ServeHTTP(writer, request)
	}
}

func (r *Root) defaultHTTPHandler(h http.Handler) http.Handler {
	tracer := lo.Must(tracing.NewAPMTracerFromConfig(tracing.NewConfigFromEnv(), r.Config.ReleaseID))
	return apmhttp.Wrap(
		loggingHandler(recoveryHandler(h), r.Logger),
		apmhttp.WithTracer(tracer),
		apmhttp.WithPanicPropagation(),
	)
}
