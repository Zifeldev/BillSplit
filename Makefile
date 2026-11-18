PROTO_DIR := api/proto
PB_OUT    := pkg/pb
SERVICES  := gateway auth social split finance notify ocr-worker

.PHONY: tools proto build test test-integration lint up down logs web secrets db-passwords

tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest

proto:
	protoc -I $(PROTO_DIR) \
		--go_out=$(PB_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PB_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/billsplit/v1/*.proto $(PROTO_DIR)/billsplit/internalapi/v1/*.proto
	# HTTP/JSON gateway only for the public API
	protoc -I $(PROTO_DIR) \
		--grpc-gateway_out=$(PB_OUT) --grpc-gateway_opt=paths=source_relative,generate_unbound_methods=true \
		$(PROTO_DIR)/billsplit/v1/*.proto

build:
	@for s in $(SERVICES); do go build -o bin/$$s ./cmd/$$s || exit 1; done

test:
	go test -race ./...

# Needs Docker (testcontainers starts Postgres/Redis).
test-integration:
	go test -tags integration ./...

lint:
	go vet ./...

# Docker secrets for compose (./secrets, git-ignored). Safe to re-run: never overwrites.
secrets:
	./scripts/gen-secrets.sh

# Apply ./secrets passwords to an existing Postgres volume (keeps data).
db-passwords:
	./scripts/db-passwords.sh

up: secrets
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f gateway auth social split finance notify ocr-worker

web:
	cd apps/mobile && npx expo start --web
