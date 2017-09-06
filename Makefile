PACKAGE    = jira-confluence-backup
DATE      ?= $(shell date +%FT%T%z)
VERSION   ?= $(shell git describe --tags --always --match=v* 2> /dev/null)
DOCKERIMG  = quay.io/unified3c/$(PACKAGE)

.PHONY: all win_amd64 linux_amd64 darwin_amd64 docker

all: win_amd64 linux_amd64 darwin_amd64

win_amd64:
	  GOARCH=amd64 GOOS=windows \
	  go build -tags release \
	  -ldflags '-X $(PACKAGE)/cmd.Version=$(VERSION) -X $(PACKAGE)/cmd.BuildDate=$(DATE)' \
	  -o bin/$(PACKAGE)_win_amd64.exe

linux_amd64:
	  GOARCH=amd64 GOOS=linux \
	  go build -tags release \
	  -ldflags '-X $(PACKAGE)/cmd.Version=$(VERSION) -X $(PACKAGE)/cmd.BuildDate=$(DATE)' \
	  -o bin/$(PACKAGE)_linux_amd64

darwin_amd64:
	  GOARCH=amd64 GOOS=darwin \
	  go build -tags release \
	  -ldflags '-X $(PACKAGE)/cmd.Version=$(VERSION) -X $(PACKAGE)/cmd.BuildDate=$(DATE)' \
	  -o bin/$(PACKAGE)_darwin_amd64

docker: linux_amd64
		docker build . \
			--tag $(DOCKERIMG):$(VERSION) \
			--tag $(DOCKERIMG):latest
		docker push $(DOCKERIMG)
