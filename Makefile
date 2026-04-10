# Makefile for docker-ui
#
# Build targets:
#   make build        # Build Docker image for linux/amd64 (default)
#   make build IMG=xxx # Build with custom image name
#   make push         # Push Docker image to registry
#   make run          # Run container (requires AUTH_USER and AUTH_PASS env vars)
#   make stop         # Stop and remove container
#   make clean        # Remove image

# Default values
IMG ?= ghcr.io/xusenlin/docker-ui:latest

.PHONY: build
build: ## Build Docker image (IMG=xxx)
	docker build -t $(IMG) .

.PHONY: push
push: ## Push Docker image to registry
	docker push $(IMG)

.PHONY: run
run: ## Run container (set AUTH_USER and AUTH_PASS env vars, or update defaults below)
	docker run -d --name docker-ui \
		-v /var/run/docker.sock:/var/run/docker.sock:ro \
		-p 8080:8080 \
		-e AUTH_USER=admin \
		-e AUTH_PASS=your_password \
		$(IMG)

.PHONY: stop
stop: ## Stop and remove container
	docker stop docker-ui || true
	docker rm docker-ui || true

.PHONY: clean
clean: ## Remove image
	docker rmi $(IMG) || true

