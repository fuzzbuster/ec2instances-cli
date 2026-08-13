.PHONY: all fmt-check vet test build check validate-skill build-local release-archives

all: check

fmt-check:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...

test:
	go test ./...

build:
	go build ./...

check: fmt-check vet test build validate-skill

validate-skill:
	test -f skills/ec2instances/SKILL.md
	head -20 skills/ec2instances/SKILL.md | grep -q '^name: "ec2instances"$$'
	head -20 skills/ec2instances/SKILL.md | grep -Eq '^description: ".+"$$'

build-local:
	go build -trimpath -o ec2instances .

release-archives:
	test -n "$$GITHUB_REF_NAME"
	test -n "$$GITHUB_SHA"
	bash -eu -o pipefail -c '\
		version="$${GITHUB_REF_NAME#scraper-v}"; \
		build_date="$$(date -u +%Y-%m-%dT%H:%M:%SZ)"; \
		mkdir -p dist; \
		for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
			os="$${target%/*}"; \
			arch="$${target#*/}"; \
			package_dir="build/$${os}-$${arch}"; \
			mkdir -p "$$package_dir"; \
			CGO_ENABLED=0 GOOS="$$os" GOARCH="$$arch" go build \
				-trimpath \
				-ldflags "-s -w -X main.version=$$version -X main.commit=$$GITHUB_SHA -X main.buildDate=$$build_date" \
				-o "$$package_dir/ec2instances" .; \
			cp LICENSE "$$package_dir/LICENSE"; \
			tar -C "$$package_dir" -czf "dist/ec2instances_$${version}_$${os}_$${arch}.tar.gz" \
				ec2instances LICENSE; \
		done; \
		if command -v sha256sum >/dev/null 2>&1; then \
			(cd dist && sha256sum ./*.tar.gz > checksums.txt); \
		else \
			(cd dist && shasum -a 256 ./*.tar.gz > checksums.txt); \
		fi'
