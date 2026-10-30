.PHONY: build build-all run-server run-satellite run-vsatellite test validate clean

# Build all components
build-all:
	$(MAKE) -C components/saas-control-plane build
	$(MAKE) -C components/satellite-gateway build
	$(MAKE) -C components/vsatellite build

# Individual builds
build-server:
	$(MAKE) -C components/saas-control-plane build

build-satellite:
	$(MAKE) -C components/satellite-gateway build

build-vsatellite:
	$(MAKE) -C components/vsatellite build

# Run
run-server:
	$(MAKE) -C components/saas-control-plane run

run-satellite:
	$(MAKE) -C components/satellite-gateway run

run-vsatellite:
	$(MAKE) -C components/vsatellite run

# Test all
test:
	$(MAKE) -C components/saas-control-plane test
	$(MAKE) -C components/satellite-gateway test
	$(MAKE) -C components/vsatellite test

validate:
	$(MAKE) -C components/saas-control-plane validate
	$(MAKE) -C components/satellite-gateway validate
	$(MAKE) -C components/vsatellite validate

# Clean all
clean:
	$(MAKE) -C components/saas-control-plane clean
	$(MAKE) -C components/satellite-gateway clean
	$(MAKE) -C components/vsatellite clean

# Docker
docker-build:
	$(MAKE) -C components/saas-control-plane docker-build
	$(MAKE) -C components/satellite-gateway docker-build

# Database
migrate:
	$(MAKE) -C components/saas-control-plane migrate
