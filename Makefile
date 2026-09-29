build:
	docker compose build --no-cache
run:
	docker compose up -d --scale backend=$(COUNT)
test:
	cd ./loadbalancer && go test -v && cd ../admin && go test -v
	

