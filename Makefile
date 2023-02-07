up-grpcui:
	docker compose up grpcui

up:
	docker compose up -d --build

up-with-debug:
	docker compose -f docker-compose.debug.yaml up -d --build

down:
	docker compose down

ps:
	docker compose ps

bld:
	cd cmd/offer-read && go build -o ../../build/binary

# Distributive
# Build distributive
define build_dist
	mkdir -p build/distr/$(1)-$(2) && \
	cd cmd/offer-read && \
	CGO_ENABLED=0 GOOS=$(1) GOARCH=$(2) \
	go build -a -installsuffix cgo -ldflags="-w -s" -o ../../build/distr/$(1)-$(2)/binary && \
	cd ../..
endef

dist:
	rm -rf build/distr build/distr_atr
	mkdir -p build/distr build/distr_art
	$(call build_dist,$(PLATFORM),$(ARCH))