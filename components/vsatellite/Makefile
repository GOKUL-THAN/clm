.PHONY: build run test vet validate clean

BINARY=bin/clm-vsatellite

build:
	../shared/scripts/go-staged.sh vsatellite build

run:
	go run ./cmd/clm-vsatellite

test:
	../shared/scripts/go-staged.sh vsatellite test

vet:
	../shared/scripts/go-staged.sh vsatellite vet

validate: test vet build

clean:
	rm -rf bin/
