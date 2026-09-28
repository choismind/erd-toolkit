build:
	go build -o erdtool.exe ./cmd/erdtool

test:
	go test ./...

# cover는 CLI 계층까지 포함한 커버리지를 잰다. `go test ./...`만으로는
# cmd/erdtool의 run* 함수가 하나도 안 잡힌다 — os.Exit을 직접 부르므로
# 단위 테스트가 부를 수 없기 때문이다. 자세한 것은 스크립트 머리에 있다.
cover:
	bash scripts/coverage.sh

release:
	bash scripts/build-release.sh $(VERSION)

.PHONY: build test cover release
