GOPKGS := ./src/...
comma:= ,
empty:=
space:= $(empty) $(empty)
GOCOMMAPKGS := $(subst $(space),$(comma),$(GOPKGS))

VERSION=$(shell cat VERSION)

all:
	go install -v ./cmd/...

test:
	go test -v ${GOPKGS}

updatedeps:
	go get -u github.com/russellhaering/hyperglide
	hyperglide update
	find vendor -type f -name *.go | xargs sed -i '' -e 's/\"github.com\/Sirupsen\/logrus/\"github.com\/sirupsen\/logrus/g'

docker:
	docker build -t us.gcr.io/sft-private-images/sft-deploy-builder . && gcloud docker -- push us.gcr.io/sft-private-images/sft-deploy-builder:latest

coverage:
	go get github.com/axw/gocov/gocov
	go get gopkg.in/matm/v1/gocov-html
	gocov test ${GOPKGS} -coverpkg ${GOCOMMAPKGS} > coverage.json
	gocov-html coverage.json > coverage.html

.PHONY: all test coverage updatedeps
