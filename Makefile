BINARIES := bin/plugs_controller
BINARIES_RPI := arm64/plugs_controller
DEPLOY_HOST := 10.56.31.3

all: build build-rpi

build: $(subst /,-,$(BINARIES))

build-rpi: $(subst /,-,$(BINARIES_RPI))

bin-plugs_controller:
	go build -o bin/plugs_controller cmd/plugs_controller/main.go

arm64-plugs_controller:
	GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc CGO_ENABLED=1 go build -o arm64/plugs_controller cmd/plugs_controller/main.go

test:
	# go test

deploy: test build-rpi
	ssh $(DEPLOY_HOST) 'sudo systemctl stop plugs_controller.service; [ -d bin ] || mkdir bin; [ -d plugs ] && rm -rf plugs/web || mkdir plugs'
	scp $(BINARIES_RPI) $(DEPLOY_HOST):bin/
	scp -r web $(DEPLOY_HOST):plugs/
	ssh $(DEPLOY_HOST) sudo systemctl start plugs_controller.service

clean:
	rm -f bin/* arm64/*
