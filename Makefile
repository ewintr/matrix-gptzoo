docker-push:
	docker build . -t matrix-bots
	docker tag matrix-bots registry.ewintr.nl/matrix-bots
	docker push registry.ewintr.nl/matrix-bots