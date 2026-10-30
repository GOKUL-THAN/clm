.PHONY: build run test vet validate clean docker-build

BINARY=bin/clm-server
CLI_BINARY=bin/clm

build:
	../shared/scripts/go-staged.sh control build

run:
	go run ./cmd/clm-server

test:
	../shared/scripts/go-staged.sh control test

vet:
	../shared/scripts/go-staged.sh control vet

validate: test vet build

clean:
	rm -rf bin/

docker-build:
	docker build -f deploy/Dockerfile.server -t m2p/clm-server:latest .

migrate:
	psql -v ON_ERROR_STOP=1 -U clm -d clm_db -f deploy/migrations/001_initial.sql
	psql -v ON_ERROR_STOP=1 -U clm -d clm_db -f deploy/migrations/002_sso_fields.sql
	psql -v ON_ERROR_STOP=1 -U clm -d clm_db -f deploy/migrations/003_tenant_security.sql
	psql -v ON_ERROR_STOP=1 -U clm -d clm_db -f deploy/migrations/004_secure_command_transport.sql
