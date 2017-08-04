
GO := go


all: build

release: format build

build:
	$(GO) build predixy_exporter.go

format:
	$(GO) fmt predixy_exporter.go

clean:
	@rm -rf predixy_exporter
	@echo Done.
