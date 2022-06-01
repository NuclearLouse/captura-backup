VERSION = 1.0.0
LDFLAGS = -ldflags "-X captura-backup/internal/service.version=${VERSION} -X captura-backup/internal/service.configFolder=configs"
LDFLAGS_DEV = -ldflags "-X captura-backup/internal/service.version=${VERSION} -X captura-backup/internal/service.configFolder=configs-dev"
DATE = $(shell date /t)

.PHONY: build_dev
build_dev:
	go build ${LDFLAGS_DEV} -mod vendor -v ./main/captura-backup

.PHONY: build
build:
	go build ${LDFLAGS} -mod vendor -v ./main/captura-backup

.PHONY: git
git:
	git a 
	git co "${DATE}"
	git pusm
#	git pusn

.DEFAULT_GOAL := build_dev