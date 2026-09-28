// internal/buildinfo/buildinfo.go
//
// 릴리스 버전 하나를 두는 자리다. 예전에는 cmd/erdtool의 패키지 변수였는데,
// 산출물 머리에 «무엇으로 뽑았는지»를 적으려면 리포터가 그 값을 봐야 한다 —
// cmd는 internal 패키지들이 볼 수 없는 위쪽이라 거기 두면 닿지 않는다.
package buildinfo

// Version은 릴리스 빌드에서 ldflags로 박는다:
//
//	go build -ldflags "-X erdtool/internal/buildinfo.Version=1.0.0" ./cmd/erdtool
//
// scripts/build-release.sh가 그렇게 부른다. 개발 빌드는 아래 기본값으로 남는다.
var Version = "0.1.0-dev"
