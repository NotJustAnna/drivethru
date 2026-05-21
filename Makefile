# drivethru — StaticSite operator for Garage + Traefik.

IMG ?= ghcr.io/notjustanna/drivethru:latest

.PHONY: build test vet fmt run docker-build install uninstall deploy undeploy

build:
	go build -o bin/manager ./cmd

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run: build
	./bin/manager --leader-elect=false

docker-build:
	docker build -t $(IMG) .

# Cluster operations. Requires `kubectl` and a configured context.
install:
	kubectl apply -k config/crd

uninstall:
	kubectl delete -k config/crd

deploy:
	cd config/manager && \
	  kubectl create configmap drivethru-config --dry-run=client -o yaml \
	    --from-literal=GARAGE_HOST=garage.garage-system.svc.cluster.local | \
	  kubectl apply -f - || true
	kubectl apply -k config/default

undeploy:
	kubectl delete -k config/default
