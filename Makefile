build:
	docker compose build --no-cache
run:
	docker compose up -d --scale backend=$(COUNT)