.PHONY: all deps test build run benchmark cover

PACKAGES = $(shell go list ./... | grep -v /examples/)

all: deps test build

deps:
	go get -u github.com/gin-gonic/gin
	go get -u github.com/spf13/viper
	go get -u github.com/op/go-logging
	go get -u github.com/gorilla/mux

test:
	go fmt ./...
	go test -cover $(PACKAGES)
	go vet ./...

benchmark:
	go test -bench=. -benchtime=3s $(PACKAGES)

build: build_gin_example build_mux_example build_gorilla_example build_negroni_example

build_gin_example:
	cd examples/gin/ && make && cd ../.. && cp examples/gin/krakend_gin_example* .

build_mux_example:
	cd examples/mux/ && make && cd ../.. && cp examples/mux/krakend_mux_example* .

build_gorilla_example:
	cd examples/gorilla/ && make && cd ../.. && cp examples/gorilla/krakend_gorilla_example* .

build_negroni_example:
	cd examples/negroni/ && make && cd ../.. && cp examples/negroni/krakend_negroni_example* .

coveralls: all
	go get github.com/mattn/goveralls
	sh coverage.sh --coveralls
