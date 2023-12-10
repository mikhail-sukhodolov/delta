package bootstrap

// Импорт необходимых пакетов
import (
	// Стандартные пакеты и пакеты для работы с HTTP
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"

	// Библиотеки для логирования, мониторинга и трассировки
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/lo"
	"gitlab.int.tsum.com/core/libraries/corekit.git/healthcheck"
	"gitlab.int.tsum.com/core/libraries/corekit.git/observability/tracing"
	"gitlab.int.tsum.com/preowned/simona/delta/core.git/log_key"
	"go.uber.org/zap"

	// Локальные пакеты
	"offer-read-service/internal/service"
)

// Функция initHTTPServer инициализирует HTTP сервер
func (r *Root) initHTTPServer() {
	mux := http.NewServeMux()

	// Инициализация pprof для профилирования
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	// Инициализация Prometheus для сбора метрик
	mux.Handle("/metrics", promhttp.Handler())

	// Инициализация эндпоинта для проверки здоровья сервиса
	mux.Handle("/health", healthcheck.Handler(
		healthcheck.WithReleaseID(r.Config.ReleaseID),
	))

	// Дополнительный обработчик HTTP
	mux.Handle("/full_index", r.defaultHTTPHandler(fullIndexHandler(r.Services.Indexator)))

	// Настройка HTTP сервера
	r.Infrastructure.HTTP = &http.Server{
		Handler:     mux,
		Addr:        r.Config.HTTP.ListenAddr,
		IdleTimeout: r.Config.HTTP.KeepaliveTime + r.Config.HTTP.KeepaliveTimeout,
	}

	// Регистрация фоновой задачи и обработчика остановки для HTTP сервера
	r.RegisterBackgroundJob(func() error {
		return r.Infrastructure.HTTP.ListenAndServe()
	})
	r.RegisterStopHandler(func() { _ = r.Infrastructure.HTTP.Shutdown(context.Background()) })
}

// Функция fullIndexHandler обрабатывает запросы на индексацию
func fullIndexHandler(indexator service.Indexator) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Запуск индексации в отдельной горутине
		go func() {
			// Получение и изменение контекста запроса для включения логгера
			// request.Context(): Этот вызов возвращает контекст, связанный с HTTP-запросом. Контекст используется в Go
			// для передачи информации, такой как данные о таймаутах или отмене операций, между разными частями программы,
			// в частности, между разными HTTP-запросами и обработчиками.
			// apm.DetachedContext(request.Context()): Функция DetachedContext из пакета go.elastic.co/apm создает новый контекст,
			// который "отсоединен" от текущей транзакции APM (Elastic APM). Это означает, что действия, выполняемые в этом контексте,
			// не будут автоматически связаны с текущей транзакцией APM. Это полезно, например, для фоновых задач,
			// которые не должны влиять на метрики производительности основного запроса.
			// ctxzap.Extract(request.Context()): Функция Extract из пакета github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap
			// извлекает экземпляр логгера zap из контекста запроса. Логгер zap - это высокопроизводительный логгер, который обеспечивает
			// структурированное логирование.
			// ctxzap.Extract(request.Context()).Named("full_index"): Метод Named добавляет указанное имя ("full_index") к логгеру, чтобы все сообщения,
			// зарегистрированные с этим логгером, включали это имя. Это помогает идентифицировать логи, связанные с определенной частью кода или функциональностью.
			// ctxzap.ToContext(apm.DetachedContext(request.Context()), ...): Функция ToContext из пакета ctxzap устанавливает логгер zap в новый контекст
			// (в данном случае, отсоединенный контекст APM), который затем можно передать в другие части приложения, чтобы обеспечить единообразное логирование.
			// Итак, эта строка кода создает новый контекст, отсоединенный от текущей транзакции APM, и устанавливает в него структурированный логгер zap
			// с указанным именем. Этот контекст затем используется для последующих операций, связанных с этим запросом, позволяя логировать эти операции
			// в единообразном и организованном виде.
			ctx := ctxzap.ToContext(apm.DetachedContext(request.Context()), ctxzap.Extract(request.Context()).Named("full_index"))

			ctxzap.Info(ctx, "indexator is starting") // Логирование начала индексации
			result, err := indexator.Index(ctx)       // Вызов функции индексации
			if err != nil {
				ctxzap.Error(ctx, "couldn't indexing", zap.Error(err)) // Логирование ошибки, если она произошла
				return
			}
			// Логирование успешного завершения индексации
			ctxzap.Info(ctx, fmt.Sprintf("indexator finished %+v", fmt.Sprintf("%+v", result)))
		}()

		writer.WriteHeader(200) // Отправка HTTP статуса 200 OK
	})
}

// Функция loggingHandler добавляет логирование к HTTP запросам
func loggingHandler(h http.Handler, logger *zap.Logger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()                       // Получение контекста запроса
		transaction := apm.TransactionFromContext(ctx) // Получение транзакции APM из контекста
		// Добавление информации о трассировке в контекст
		ctx = ctxzap.ToContext(ctx, logger.With(zap.String("trace.id", transaction.TraceContext().Trace.String())))
		request = apmhttp.RequestWithContext(ctx, request) // Обновление контекста в запросе
		h.ServeHTTP(writer, request)                       // Вызов оригинального обработчика с обновленным контекстом
	}
}

// Функция recoveryHandler обрабатывает паники в HTTP запросах
func recoveryHandler(h http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				// Логирование паники и стека вызовов
				ctxzap.Error(request.Context(), "panic", zap.Any(log_key.PanicReason, r), zap.Stack("stacktrace"))
			}
		}()
		h.ServeHTTP(writer, request) // Вызов оригинального обработчика
	}
}

// Функция defaultHTTPHandler добавляет трассировку, логирование и обработку паник к HTTP обработчику
func (r *Root) defaultHTTPHandler(h http.Handler) http.Handler {
	tracer := lo.Must(tracing.NewAPMTracerFromConfig(tracing.NewConfigFromEnv(), r.Config.ReleaseID))
	// Оборачивание обработчика в APM, логгер и обработчик паник
	return apmhttp.Wrap(
		loggingHandler(recoveryHandler(h), r.Logger),
		apmhttp.WithTracer(tracer),
		apmhttp.WithPanicPropagation(),
	)
}
