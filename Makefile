
GO := go

target = predixy_exporter

all: build

release: format build

build: $(target)

$(target): exporter/exporter.go $(target).go
	$(GO) build $(target).go

format:
	$(GO) fmt exporter/exporter.go
	$(GO) fmt $(target).go

clean:
	@rm -rf $(target)
	@echo Done.
