package main

import (
	// Импорт стандартных и внешних библиотек
	"context"                                                                               // Контекст для управления жизненным циклом процессов и горутин
	"gitlab.int.tsum.com/preowned/libraries/go-gen-proto.git/v3/gen/utp/offer_read_service" // Импорт сгенерированного gRPC сервиса
	"log"                                                                                   // Библиотека для логирования
	"offer-read-service/internal/grpcserver"                                                // Локальный пакет для gRPC сервера
	"os/signal"                                                                             // Библиотека для обработки сигналов ОС
	"syscall"                                                                               // Библиотека для работы с системными вызовами

	"offer-read-service/internal/bootstrap" // Локальный пакет для инициализации приложения
)

// releaseID устанавливается во время сборки в CI/CD пайплайне с использованием ldflags
// (например: go build -ldflags="-X 'main.releaseID=<release id>'").
var releaseID string

func main() {
	// Создание контекста с обработкой системных сигналов SIGINT и SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel() // Отложенный вызов функции отмены контекста

	// Создание новой конфигурации приложения
	config, err := bootstrap.NewConfig()
	if err != nil {
		log.Panicf("can't create new config.go: %v", err) // Логирование ошибки и завершение программы в случае ошибки
	}
	config.ReleaseID = releaseID // Установка идентификатора релиза в конфигурацию

	// Инициализация корневого объекта приложения с передачей контекста и конфигурации
	root, err := bootstrap.NewRoot(ctx, config)
	if err != nil {
		log.Panicf("application could not been initialized: %v", err) // Логирование ошибки и завершение программы в случае ошибки
	}

	// Регистрация gRPC сервера
	offer_read_service.RegisterOfferReadServiceServer(root.Server, grpcserver.NewServer(root))

	// Запуск приложения
	if err = root.Run(ctx); err != nil {
		log.Panicf("application terminated abnormally: %s", err) // Логирование ошибки и завершение программы в случае ошибки
	}
}
