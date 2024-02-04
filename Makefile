BINARIES := bin/plugs_controller
BINARIES_RPI := arm64/plugs_controller

all: build build-rpi

build: $(subst /,-,$(BINARIES))

build-rpi: $(subst /,-,$(BINARIES_RPI))

bin-plugs_controller:
	go build -o bin/plugs_controller cmd/plugs_controller/main.go

arm64-plugs_controller:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc go build -o arm64/plugs_controller cmd/plugs_controller/main.go

test:
	# go test

deploy: test build-rpi
	ssh pi3 'sudo systemctl stop plugs_controller.service; [ -d bin ] || mkdir bin; [ -d plugs ] || mkdir plugs'
	scp $(BINARIES_RPI) pi3:bin/
	scp web/root.html pi3:plugs/
	ssh pi3 sudo systemctl start plugs_controller.service

clean:
	rm -f bin/* arm64/*
