# Указываем базовый образ для сборочной стадии с Golang и Alpine Linux
FROM golang:1.18.1-alpine3.15 AS builder

# Обновляем индекс пакетов и устанавливаем Git, не сохраняя кеш пакетов
RUN apk update && apk add --no-cache git

# Устанавливаем инструмент grpcui, который предоставляет GUI для тестирования gRPC-серверов
RUN go install github.com/fullstorydev/grpcui/cmd/grpcui@v1.3.0

# Копируем скрипт запуска в контейнер и делаем его исполняемым
COPY start.sh /usr/local/scripts/
RUN chmod +x /usr/local/scripts/*.sh

# Указываем базовый образ для финальной стадии
FROM alpine:3.15.4

# Копируем скрипты и бинарный файл grpcui из сборочного образа
COPY --from=builder /usr/local/scripts /usr/local/scripts
COPY --from=builder /go/bin/grpcui /usr/bin/grpcui

# Обновляем индекс пакетов и устанавливаем bash, не сохраняя кеш пакетов
RUN apk update && apk add --no-cache bash

# Создаем нового пользователя с заданным UID и переключаемся на него
RUN adduser -S -u 10001 user
USER user

# Устанавливаем скрипт start.sh как точку входа
CMD ["/usr/local/scripts/start.sh"]
