# Указываем базовый образ для сборочной стадии
FROM repo.int.tsum.com/tsum/core/golang:1.18 as builder

# Добавляем ключи хоста GitLab в список известных хостов для предотвращения проблем с подключением
RUN mkdir -p -m 0600 ~/.ssh && ssh-keyscan gitlab.int.tsum.com >> ~/.ssh/known_hosts

# Настраиваем Git для перенаправления https запросов на ssh
RUN git config --global url.'git@gitlab.int.tsum.com:'.insteadOf 'https://gitlab.int.tsum.com/'

# Устанавливаем рабочую директорию в контейнере
WORKDIR /app

# Копируем файлы go.mod и go.sum в рабочую директорию контейнера
COPY ./go.mod ./go.sum .

# Загружаем зависимости проекта, используем Docker cache mount для кеширования зависимостей
RUN --mount=type=ssh --mount=type=cache,id=go_cache,target=/go/pkg/mod go mod download

# Копируем все файлы проекта в рабочую директорию контейнера
COPY . .

# Собираем приложение, используем Docker cache для кеширования сборочных файлов и папки кеша Go
RUN --mount=type=cache,id=go_cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache CGO_ENABLED=0 GOOS=linux go build -v ./cmd/offer-read/main.go

# Указываем базовый образ для финальной стадии
FROM alpine:3.15.4

# Копируем скомпилированный файл приложения из сборочного контейнера
COPY --from=builder /app/main main

# Указываем команду для запуска приложения
CMD ["./main"]
