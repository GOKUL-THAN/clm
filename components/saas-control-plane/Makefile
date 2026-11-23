.PHONY: build run test vet validate clean docker-build migrate migrate-baseline

BINARY=bin/clm-server
CLI_BINARY=bin/clm

build:
	bash ../shared/scripts/go-staged.sh control build

run:
	go run ./cmd/clm-server

test:
	bash ../shared/scripts/go-staged.sh control test

vet:
	bash ../shared/scripts/go-staged.sh control vet

validate: test vet build

clean:
	rm -rf bin/

docker-build:
	docker build -f deploy/Dockerfile.server -t m2p/clm-server:latest .

migrate:
	bash ../shared/scripts/go-staged.sh control migrate

migrate-baseline:
	CLM_BASELINE_LEGACY=true bash ../shared/scripts/go-staged.sh control migrate
