#!/usr/bin/env bash
# scripts/coverage.sh — CLI 계층까지 포함한 커버리지를 잰다.
#
# 왜 이 스크립트가 있나: cmd/erdtool의 run* 함수들은 os.Exit을 직접 부르므로
# 단위 테스트가 부를 수 없다(부르면 테스트 프로세스가 죽는다). 그 아홉이
# 파일의 70%인데, 그 안에 usage 문구·종료 코드·못 쓰는 플래그 조합 같은
# «사용자가 가장 먼저 만나는 것»이 들어 있다.
#
# 얼마나 안 잡히나: `go test -cover ./cmd/erdtool`만 돌리면 30.0%다
# (2026-09-22 실측). 이 스크립트까지 돌리면 87.8%가 된다.
#
# 그래서 Go 1.20+의 `go build -cover`를 쓴다. 그렇게 빌드한 바이너리는
# GOCOVERDIR에 커버리지를 쓰므로, 실제로 돌린 명령이 그대로 측정된다.
# 그 결과를 단위 테스트가 모은 것과 합쳐 하나로 낸다.
# 설계 원칙 4(「exit 0이 곧 정답이 아니다 — 실제 바이너리를 빌드해 돌린다」)를
# 숫자로 재는 셈이다.
#
# 서버는 필요 없다. 남의 기계에서도, CI에서도 그대로 돈다.
set -euo pipefail

# Go 도구(covdata)는 Git Bash의 /d/... 형식을 못 읽는다. 그래서 이 스크립트는
# 경로를 처음부터 그 플랫폼이 쓰는 모양으로 잡는다 — Windows에서는 `pwd -W`가
# D:/... 를 주고, 리눅스에는 그 플래그가 없으므로 그냥 pwd다.
if (cd / && pwd -W) >/dev/null 2>&1; then
	ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -W)"
else
	ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi
cd "$ROOT"

OUT="${COVERAGE_OUT:-$ROOT/.coverage}"
BIN_DIR="$OUT/bin"    # 바이너리를 돌려 모은 것
UNIT_DIR="$OUT/unit"  # go test가 모은 것
WORK="$OUT/work"      # 명령을 돌릴 작업 폴더
rm -rf "$OUT"
mkdir -p "$BIN_DIR" "$UNIT_DIR" "$WORK"

EXE="$OUT/erdtool-cov"
[ "${OS:-}" = "Windows_NT" ] && EXE="$EXE.exe"

echo "==> 커버리지 계측 바이너리를 만든다"
go build -cover -o "$EXE" ./cmd/erdtool

# run은 명령 하나를 돌린다. 실패해도 멈추지 않는다 — 이 스크립트가 재는
# 것 중 절반이 «실패하는 길»이고, 그 길의 종료 코드가 곧 계약이다.
run() {
	GOCOVERDIR="$BIN_DIR" "$EXE" "$@" >/dev/null 2>&1 || true
}

echo "==> 픽스처를 놓는다"
cp internal/drawio/testdata/relationship_physical.drawio "$WORK/주문정보.drawio"
cp internal/drawio/testdata/entity_table_basic.drawio "$WORK/고객정보.drawio"
cp internal/drawio/testdata/floating_relation.drawio "$WORK/떠있는선.drawio"
# 일부러 깨뜨린 파일 — 폴더 처리에서 «하나가 실패해도 나머지는 뽑힌다»와
# 종료 코드 1을 재는 데 쓴다.
printf '<mxfile>' > "$WORK/깨진것.drawio"
# build가 읽을 설계서. CSV는 텍스트라 여기서 만든다(.drawio는 손으로 적지
# 않는다는 원칙이 있지만 CSV에는 해당하지 않는다).
cat > "$WORK/설계서.csv" <<'CSV'
테이블명,순번,컬럼명,컬럼유형,색인여부,널허용,단일값
사원,1,사원번호,int,PK,False,True
사원,2,부서번호,int,FK1,False,True
사원,3,사원명,char(50),,False,True
부서,1,부서번호,int,PK,False,True
부서,2,부서명,char(50),,False,True
## 관계
순번,원천테이블명,원천컬럼명,원천카디널리티,원천색인,연관명,목표카디널리티,목표색인,목표테이블명,목표컬럼명
1,부서,부서번호,ERone,PK,부서사원,ERzeroToMany,FK1,사원,부서번호
CSV

# 표준용어사전(xlsx)과 SQLite는 텍스트로 적을 수 없어 만들어 쓴다.
# 이 둘이 있어야 convert와 reverse의 «잘 되는 길»을 잰다.
go run ./scripts/covfixture "$WORK"

cd "$WORK"

echo "==> 잘 되는 길"
run version
run generate 주문정보.drawio
run generate 고객정보.drawio --relations --sql
run generate 주문정보.drawio --dialect mysql --sql
run generate . --recursive
run build 설계서.csv
run build 설계서.csv --out 다른이름.drawio
run generate 다른이름.drawio
run annotate 떠있는선.drawio
run annotate 떠있는선.drawio --clean
run annotate . --recursive
run convert 주문정보.drawio --dry-run
run convert 주문정보.drawio
run convert 주문정보.drawio            # 두 번째 — [OVERWRITTEN]
run convert 고객정보.drawio --out 물리
run convert .                          # 폴더. *.physical.drawio는 건너뛴다
run reverse shop.db
run reverse shop.db --out 그림.drawio
run generate 그림.drawio

echo "==> 잘못 쓰는 길 — 이쪽이 아무 테스트도 안 보던 자리다"
run                                   # 서브커맨드 없음
run 없는커맨드
run version --bogus                   # 인자를 안 받는다
run generate                          # 대상 없음
run generate 없는파일.drawio
run generate 주문정보.drawio --nope    # 모르는 플래그
run generate 주문정보.drawio --dialect postgre   # 오타난 타깃
run generate 주문정보.drawio --config  # 값 없는 플래그
run watch                             # 대상 없음
run watch 없는폴더
run watch . --nope
run convert                           # 대상 없음
mkdir -p 사전없음 && cp 주문정보.drawio 사전없음/
(cd 사전없음 && run convert 주문정보.drawio)   # 사전이 없다
run convert . --dry-run               # 못 쓰는 조합
run convert 주문정보.drawio --recursive
run convert 주문정보.drawio --dry-run --out 어딘가
run build                             # 대상 없음
run build 없는설계서.xlsx
run build 설계서.csv --out 설계서.csv  # 입력을 덮어쓰려 한다
run reverse                           # 대상 없음
run reverse 없는것
run reverse "oracle://u:p@localhost/db"
run reverse 설계서.csv --out 설계서.csv
run annotate                          # 대상 없음
run annotate 없는파일.drawio
run annotate . --dry-run              # 못 쓰는 조합
run annotate 주문정보.drawio --recursive

cd "$ROOT"

echo "==> 단위 테스트도 같은 형식으로 모은다"
go test ./... -cover -count=1 -args -test.gocoverdir="$UNIT_DIR" >/dev/null

echo
echo "==> 패키지별"
go tool covdata percent -i="$BIN_DIR,$UNIT_DIR"

echo
echo "==> cmd/erdtool 함수별"
go tool covdata textfmt -i="$BIN_DIR,$UNIT_DIR" -o "$OUT/merged.txt"
go tool cover -func="$OUT/merged.txt" | grep -E 'cmd/erdtool|^total'

echo
echo "HTML로 보려면: go tool cover -html=$OUT/merged.txt"
