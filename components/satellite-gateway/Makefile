.PHONY: build run test vet validate clean docker-build

BINARY=bin/clm-satellite

build:
	../shared/scripts/go-staged.sh satellite build

run:
	go run ./cmd/clm-satellite

test:
	../shared/scripts/go-staged.sh satellite test

vet:
	../shared/scripts/go-staged.sh satellite vet

validate: test vet build

clean:
	rm -rf bin/

docker-build:
	docker build -f deploy/Dockerfile.satellite -t m2p/clm-satellite:latest .
