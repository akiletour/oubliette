.PHONY: build test

build: web/dist
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o oubliette .

test: web/dist
	go vet ./...
	go test ./...

web/dist: web/node_modules web/index.html web/vite.config.ts web/tsconfig.json $(shell find web/src web/public -type f)
	npm --prefix web run build
	touch web/dist

web/node_modules: web/package-lock.json
	npm --prefix web ci
	touch web/node_modules
