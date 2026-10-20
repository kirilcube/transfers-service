up:
	docker-compose up -d

down:
	docker-compose down

reset:
	@echo "🛑 Stopping services and destroying incident data..."
	docker-compose down -v
	@echo "🧹 Cleanup complete. Database, Kafka, and metrics are wiped clean!"
	@echo "🚀 Booting up the lab again..."
	docker-compose up -d --build

clean:
	@echo "🛑 Destroying all data volumes..."
	docker-compose down -v

test:
	go test -v -race ./...