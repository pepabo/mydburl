OPENSSL_SUBJ = /C=UK/ST=Test State/L=Test Location/O=Test Org/OU=Test Unit
OPENSSL_ROOT_CA = $(OPENSSL_SUBJ)/CN=fake-CA
OPENSSL_SERVER = $(OPENSSL_SUBJ)/CN=fake-server
OPENSSL_CLIENT = $(OPENSSL_SUBJ)/CN=fake-client

default: test

ci: depsdev

test: cert
	go test ./... -coverprofile=coverage.out -covermode=count

lint:
	golangci-lint run ./...

cert: root-ca server-cert client-cert

root-ca:
	openssl genrsa 2048 > testdata/certs/root-ca-key.pem
	openssl req -new -x509 -sha512 -nodes -days 3600 -subj "$(OPENSSL_ROOT_CA)" -key testdata/certs/root-ca-key.pem -out testdata/certs/root-ca.pem

server-cert:
	openssl req -newkey rsa:2048 -sha512 -days 3600 -nodes -subj "$(OPENSSL_SERVER)" -keyout testdata/certs/server-key.pem -out testdata/certs/server-req.pem
	openssl rsa -in testdata/certs/server-key.pem -out testdata/certs/server-key.pem
	openssl x509 -sha512 -req -in testdata/certs/server-req.pem -days 3600 -CA testdata/certs/root-ca.pem -CAkey testdata/certs/root-ca-key.pem -set_serial 01 -out testdata/certs/server-cert.pem -extfile testdata/openssl.cnf
	openssl verify -CAfile testdata/certs/root-ca.pem testdata/certs/server-cert.pem

client-cert:
	openssl req -newkey rsa:2048 -sha512 -days 3600 -nodes -subj "$(OPENSSL_CLIENT)" -keyout testdata/certs/client-key.pem -out testdata/certs/client-req.pem
	openssl rsa -in testdata/certs/client-key.pem -out testdata/certs/client-key.pem
	openssl x509 -sha512 -req -in testdata/certs/client-req.pem -days 3600 -CA testdata/certs/root-ca.pem -CAkey testdata/certs/root-ca-key.pem -set_serial 01 -out testdata/certs/client-cert.pem
	openssl verify -CAfile testdata/certs/root-ca.pem testdata/certs/client-cert.pem

depsdev:
	go install github.com/Songmu/gocredits/cmd/gocredits@latest

prerelease_for_tagpr: depsdev
	go mod tidy
	gocredits -w .
	git add CHANGELOG.md CREDITS go.mod go.sum

.PHONY: default test
