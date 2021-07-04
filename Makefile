.PHONY: deploy
deploy: build-linux docker-build docker-push

.PHONY: build-linux
build-linux:
	@echo "building upcloud csi for linux"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-X main.version=$(VERSION)' -o csi-upcloud-plugin ./cmd/csi-upcloud-driver


.PHONY: docker-build
docker-build:
	@echo "building docker image to dockerhub $(REGISTRY) with version $(VERSION)"
	docker build . -t $(REGISTRY)/upcloud-csi:$(VERSION)

.PHONY: docker-push
docker-push:
	docker push $(REGISTRY)/upcloud-csi:$(VERSION)

.PHONY: test
test:
	go test -race github.com/zettavisor/upcloud-csi/driver -v