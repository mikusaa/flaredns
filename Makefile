.PHONY: dev-backend dev-frontend test build docker
dev-backend:
	cd backend && FLAREDNS_DATA_DIR=../data go run ./cmd/flaredns
dev-frontend:
	cd frontend && npm run dev
test:
	cd backend && go test ./...
	cd frontend && npm test
	cd frontend && npm run build
build:
	cd frontend && npm run build
	rm -rf backend/internal/server/web/assets
	cp frontend/dist/index.html backend/internal/server/web/index.html
	cp -R frontend/dist/assets backend/internal/server/web/assets
	cd backend && go build ./cmd/flaredns
docker:
	docker compose build
