docker-push:
	docker build . -t matrix-gptzoo:latest
	docker tag matrix-gptzoo:latest registry.ewintr.nl/matrix-gptzoo:latest
	docker push registry.ewintr.nl/matrix-gptzoo:latest