
VERSION=$(shell git describe | sed 's/^v//')
GOFILES=service
CONTAINER=gcr.io/trust-networks/probe-conf-svc:${VERSION}

all: ${GOFILES} container

GODEPS=go/.creds go/.bolt

SOURCES=service.go probe-creds.go vpn-service-creds.go

service: ${SOURCES} ${GODEPS}
	GOPATH=$$(pwd)/go go build ${SOURCES}

go/.creds:
	GOPATH=$$(pwd)/go go get github.com/trustnetworks/credentials
	touch $@

go/.bolt:
	GOPATH=$$(pwd)/go go get github.com/boltdb/bolt
	touch $@

container:
	docker build -t ${CONTAINER} .

push: container
	gcloud docker -- push ${CONTAINER}

clean:
	rm -rf ${GOFILES}
	rm -rf go

upload:
	kubectl create secret generic probe-conf-svc-creds \
	  --from-file=cert.server --from-file=key.server
	kubectl create secret generic probe-conf-svc-keys \
	  --from-file=private.json

delete:
	-kubectl delete secret probe-conf-svc-creds
	-kubectl delete secret probe-conf-svc-keys
