BINARIES := bin/plugs_controller
BINARIES_RPI := arm64/plugs_controller
DEPLOY_HOST := 10.56.31.3
DEPLOY_USER := pi

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
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) 'sudo systemctl stop plugs_controller.service; [ -d bin ] || mkdir bin'
	rsync -azv $(BINARIES_RPI) $(DEPLOY_USER)@$(DEPLOY_HOST):bin/
	ssh $(DEPLOY_USER)@$(DEPLOY_HOST) sudo systemctl start plugs_controller.service

clean:
	rm -f bin/* arm64/*
