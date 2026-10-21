up:
	docker-compose up -d

down:
	docker-compose down

reset:
	docker-compose down -v
	docker-compose up -d --build

clean:
	docker-compose down -v

test:
	go test -v -race ./...