.PHONY: up down logs test test-backup test-demo backup fmt

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f

test:
	docker build --target test .

test-backup:
	./scripts/test-backup-restore.sh

test-demo:
	./scripts/test-demo.sh

backup:
	./scripts/backup.sh

fmt:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.27-alpine gofmt -w cmd internal
