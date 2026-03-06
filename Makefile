generate-schema:
	mkdir -p schema
	cp ../../packages/types/dist/json-schema/render.json schema/render.json
	cp ../../packages/types/dist/json-schema/render-defaults.json schema/render-defaults.json

build:
	go build ./cmd/urlbox

test:
	go test ./...
