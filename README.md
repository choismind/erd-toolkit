# erdtool

**A Go CLI that reads, rewrites and draws draw.io ERD files.**

- `.drawio` → table/relation specifications (Markdown, HTML, xlsx, PDF), a validation report, SQL DDL and a JSON IR
- PostgreSQL · MySQL · SQLite → `.drawio` (queried from the live catalog, not parsed from a DDL dump)
- Spreadsheet (xlsx/csv) → `.drawio`
- Validation findings marked back onto the same `.drawio`, and removable again

**Everything the tool writes is Korean** — diagnostics, reports and specifications.
So is the rest of this README.

Built with [Claude Code](https://claude.com/claude-code), Anthropic's AI coding tool.

---

ERD를 그리고 나면 늘 같은 일이 남는다. 정의서를 손으로 옮겨 적고, DDL을
손으로 짜고, 둘이 그림과 어긋나기 시작한다. **`erdtool`은 그 셋을 `.drawio`
하나에서 뽑는다.** 반대 방향으로도 간다 — DB나 설계서에서 그림을 그린다.

```
                     generate ──→ 테이블정의서·관계정의서·검증리포트·SQL DDL·IR
                    ╱
   .drawio ERD ────┼── convert  ──→ 논리 ERD를 물리 ERD로 (표준용어사전 적용)
                    ╲
                     annotate ──→ 검증 결과를 ERD에 되반영 (제자리 수정)

   설계서(xlsx/csv) ─── build ────→ .drawio ERD
   DB 서버·파일 ────── reverse ──→ .drawio ERD

   폴더 ───────────── watch ─────→ 저장을 감지해 generate 자동 실행
```

## 받기

[릴리스 페이지](https://github.com/choismind/erd-toolkit/releases/latest)에서
플랫폼에 맞는 파일을 받는다. **설치할 것이 없다** — 받은 파일 하나가 곧
실행 파일이다.

| 파일 | 어디서 |
|---|---|
| `erdtool-windows-amd64.exe` | Windows |
| `erdtool-darwin-arm64` | macOS (Apple Silicon) |
| `erdtool-darwin-amd64` | macOS (Intel) |
| `erdtool-linux-amd64` | Linux |

함께 올라가는 `SHA256SUMS`로 받은 파일이 온전한지 확인할 수 있다.
예제는 `erdtool-examples.zip`에 들어 있다.

> [!warning] Windows에서는 SmartScreen이 막는다
> 서명이 없는 실행 파일이라 「Windows의 PC 보호」 화면이 뜬다. 바이러스라는
> 뜻이 아니다. 「추가 정보」를 누르고 「실행」을 고른다.

macOS와 Linux에서는 받은 뒤 실행 권한을 준다.

```bash
chmod +x erdtool-darwin-arm64
./erdtool-darwin-arm64 version
```

아래 예제들은 파일 이름을 `erdtool`로 줄여 적는다. 이름을 바꾸거나 PATH에
넣어 두면 그대로 쓸 수 있다.

## 첫 실행

받은 예제 안에 ERD 하나가 들어 있다. 테이블 넷, 관계 셋짜리 온라인 서점이다.

```bash
erdtool generate examples/bookstore.drawio --relations --sql
```

```
[OK] examples/bookstore.drawio -> examples\bookstore_report (4 tables)
```

`examples/bookstore_report/`에 산출물 여덟이 나온다.

| 파일 | 무엇 |
|---|---|
| `table_doc.md` · `.html` · `.xlsx` · `.pdf` | 테이블정의서 네 형식 |
| `relation_doc.md` | 관계정의서 |
| `validation_report.md` | 검증 리포트 |
| `schema.sql` | SQL DDL |
| `ir.json` | 중간표현 |

`schema.sql`은 이렇게 나온다.

```sql
CREATE TABLE "ORD" (
  "ORD_NO" int NOT NULL,
  "MBR_NO" int NOT NULL,
  "ORD_YMD" date NOT NULL,
  PRIMARY KEY ("ORD_NO"),
  FOREIGN KEY ("MBR_NO") REFERENCES "MEMBER" ("MBR_NO")
);
```

**`FOREIGN KEY` 절은 그림의 관계선에서 왔다.** 어느 쪽이 외래키인지는 키 셀의
`FK` 표기로 정한다 — 표기가 없으면 지어내지 않고 검증 리포트에 올린다.

예제를 더 뜯어보려면 [`examples/README.md`](examples/README.md)를 읽는다.
그림을 고쳐 저장하고 같은 명령을 다시 돌리면 산출물이 따라 바뀐다.

## draw.io가 필요하다

이 도구는 `.drawio` 파일을 다룰 뿐 그림을 보여 주지는 않는다. 그리거나 열려면
draw.io가 있어야 한다.

| | |
|---|---|
| [데스크톱판](https://github.com/jgraph/drawio-desktop/releases) | 설치해서 쓴다 |
| [app.diagrams.net](https://app.diagrams.net/) | 브라우저에서 바로 쓴다 |

ERD 도형은 「도형」 패널의 「ER」 카테고리에 있다. **어떻게 그려야 이 도구가
읽는지**는 [`docs/user-guide.md`](docs/user-guide.md)에 있다.

## 한눈에 보기

서브커맨드 일곱이 각각 한 가지 일만 한다.

| 커맨드 | 하는 일 | 입력 → 출력 | 자주 쓰는 것 |
|---|---|---|---|
| [`generate`](#generate--erd를-읽어-문서를-낸다) | ERD를 읽어 문서를 낸다 | `.drawio` → `<이름>_report/` | `--relations` `--sql` |
| [`convert`](#convert--논리-erd--물리-erd) | 한글 논리명을 영문 물리명으로 (표준용어사전) | `X.drawio` → `X.physical.drawio` | `--dictionary` `--dry-run` |
| [`build`](#build--데이터-설계서--erd) | 설계서를 읽어 ERD를 그린다 | `.xlsx`·`.csv` → `.drawio` | `--out` |
| [`reverse`](#reverse--db-역공학--erd) | DB에 접속해 ERD를 그린다 | PostgreSQL·MySQL·SQLite → `.drawio` | `--connections` |
| [`annotate`](#annotate--검증-결과를-erd에-되반영) | 검증 결과를 ERD에 되반영 **(제자리 수정)** | `.drawio` → 같은 `.drawio` | `--clean` `--dry-run` |
| [`watch`](#watch--저장-감지-자동-실행) | 저장을 감지해 `generate` 자동 실행 | 폴더 → 저장할 때마다 산출물 | `--recursive` |
| [`version`](#version) | 버전 출력 | — | — |

`--config`와 `--recursive`는 `generate`·`convert`·`annotate`·`watch` 넷이
공통으로 받는다. 모르는 플래그는 **어느 커맨드든 거부한다** — 조용히
무시하고 성공을 보고하지 않는다.

그 밖에: [검증 규칙](#검증-규칙) · [제약 적기](#제약-적기--erd_-속성) ·
[설정 파일](#설정-파일) · [설계 원칙](#설계-원칙) · [더 읽을 문서](#문서)

## 서브커맨드

### `generate` — ERD를 읽어 문서를 낸다

```bash
erdtool generate <file-or-dir> [--config path] [--dialect name] [--relations] [--sql] [--recursive]
```

입력 파일 옆에 `<이름>_report/` 폴더를 만들고 산출물을 쓴다.

| 파일 | 내용 | 조건 |
|---|---|---|
| `table_doc.md` · `.html` · `.xlsx` · `.pdf` | 테이블정의서 네 형식 | 항상 |
| `validation_report.md` | 검증 리포트 | 항상 |
| `ir.json` | 중간표현 — 다른 도구가 물어 쓰는 자리 | 항상 |
| `relation_doc.md` | 관계정의서 | `--relations` |
| `schema.sql` | SQL DDL | `--sql` |

PDF는 한글 폰트를 바이너리에 넣고 다닌다 — 시스템 폰트에 기대지 않는다.

#### `--dialect` — 어느 DBMS를 겨누는가

```bash
erdtool generate sakila.drawio --dialect mysql --sql
```

`ansi`(기본) · `postgres` · `mysql` · `sqlite`를 받는다. `standard`·`sql`·
`postgresql`·`pg`·`mariadb`·`sqlite3` 같은 표기도 같은 것으로 읽는다. **모르는 값은 거부한다** —
`--dialect postgre` 같은 오타가 조용히 기본값으로 흘러가면 사용자는 자기가
지정한 대로 뽑혔다고 믿는다.

**`--sql`과 독립이다.** `--dialect`만 줘도 뜻이 있다.

| 준 것 | 결과 |
|---|---|
| (없음) | ANSI 기준으로 검사. 문서에 「ANSI (지정 안 함)」 |
| `--dialect mysql` | MySQL 기준. 문서에 「MySQL」. `schema.sql`은 안 나옴 |
| `--sql` | ANSI로 `schema.sql` |
| `--sql --dialect mysql` | MySQL 문법으로 `schema.sql` (백틱 인용) |

DDL 자체는 `--sql` 없이도 내부에서 만들어진다 — 문법 검사(`ddl_syntax`)가 그것을
SQL 엔진에 먹여 보기 때문이다. `--sql`은 그 DDL을 파일로도 남길지만 정한다.

타깃은 **산출물에 박힌다.** 정의서 머리의 「타깃 DBMS」 줄과 `schema.sql`
첫 줄의 `-- 타깃 DBMS: MySQL`이다. 안 박으면 나중에 그 파일만 보고는 어느
DB용인지 알 수 없다.

설정 파일에도 `dialect:`로 적을 수 있고, `--dialect`가 그 실행 1회에 한해
덮어쓴다. `watch`는 플래그를 매번 줄 수 없으니 설정 파일 쪽이 맞는 자리다.

**아직 타입은 안 바꾼다.** 인용부호와 표기만 타깃 DBMS를 따르고,
`smallint unsigned` 같은 특정 DBMS 전용 타입은 그대로 나간다. 타입 매핑은
«바꿨다»는 사실을 전부 리포트에 남겨야 해서 따로 할 일이다.

#### 테이블정의서 레이아웃

네 형식이 같은 레이아웃을 쓴다. **문서 머리 → 테이블 일람 → 테이블마다 한 자리**.

**테이블 하나가 문서의 한 자리**다. PDF는 한 페이지, 엑셀은 한 탭, Markdown·
HTML은 한 절이다. 그 자리 안에 컬럼 표와 **그 테이블에 걸린 관계**가 함께
들어간다 — 정의서를 넘기다 어느 테이블에서 멈추면 거기서 둘 다 볼 수 있어야
한다. 관계를 `--relations` 파일로만 내보내면 기본 산출물만 받은 사람은 ERD의
절반을 못 본다.

문서 머리에는 원본 파일, 기준 시각, 생성 도구 버전, 테이블 수가 들어간다.
기준 시각은 «뽑은 시각»이 아니라 **원본 `.drawio`가 마지막으로 바뀐 시각**이다.
뽑은 시각을 쓰면 같은 ERD로 두 번 돌린 산출물이 서로 다른 바이트가 되어
`watch`를 켜 둔 동안 계속 어긋나고, 「문서가 바뀌었나」를 diff로 볼 수 없다.

컬럼 표의 열은 열이다.

| 열 | 값 |
|---|---|
| No | 표 안의 순번 |
| 컬럼명 · 데이터 타입 | 정의 셀에서 읽은 것 |
| PK | 기본키면 `●` |
| FK | 키 셀의 원문(`FK1`) — 어느 관계에 딸린 외래키인지가 그 번호에 있다 |
| 참조 | 그 외래키가 가리키는 `테이블.컬럼` |
| NULL 허용 | `Y` / `N` |
| 기본값 | `erd_default` |
| 제약 | `UNIQUE`, `CHECK (…)`, 그리고 PK도 FK도 아닌 키 셀 원문 |
| 설명 | `erd_comment` |

「참조」 칸은 관계선에서 온다. **어느 쪽이 외래키인지는 키 셀의 `FK` 표기로
정한다** — 한쪽만 FK면 그쪽이 자식이다. 둘 다이거나 둘 다 아니면 판정하지
않고 `relationship_without_fk`로 진단을 낸다. 지어내면 정의서가 「A가 B를
참조한다」고 단언하는데 실제로는 반대일 수 있고, 문서를 읽는 사람은 그것을
의심할 근거가 없다.

키 셀에 `PK`를 `P K`로 잘못 친 값도 「제약」 칸에 `키 셀: "P K"`로 남는다.
PK/FK 두 칸으로 나눠 담으면 그런 값이 표에서 아예 사라지는데, 사라지면
사용자는 자기가 그 칸에 무엇을 썼는지 문서만 보고는 알 수 없다.

관계 표는 «이 테이블 기준»이다.

| 열 | 값 |
|---|---|
| No | 표 안의 순번 |
| 방향 | `→ 참조함` / `← 참조됨` / `미정` |
| 상대 테이블 | 관계선 저쪽 끝의 테이블 |
| 이 테이블 컬럼 · 상대 컬럼 | 양 끝의 컬럼 |
| 카디널리티(이쪽) · (상대) | 사람 말 + draw.io 원본 코드 |

같은 관계가 양쪽 테이블에 한 줄씩 나온다 — 어느 쪽을 펴 봐도 보여야 한다.
`미정`은 어느 쪽이 외래키인지 판정하지 못한 것이며, **그래도 표에서 빼지
않는다.** 조용히 빠지면 그림에는 선이 있는데 문서에는 흔적도 없다.

엑셀은 **테이블 하나에 시트 하나**이고 맨 앞이 「목차」 시트다. 목차의
테이블명을 누르면 그 시트로 건너뛴다. 머리글 행은 틀 고정되고, 관계 표는
같은 탭 안에 이어 붙는다 — 별도 시트로 빼면 테이블 하나를 보려고 탭 둘을
오가야 한다.

`--relations`의 `relation_doc.md`는 **파일 전체의 관계를 한 표로** 보는
용도로 남는다. 테이블별 관계는 이제 정의서에 항상 들어간다.

관계정의서(`--relations`)의 카디널리티 칸은 `0..N (없거나 여럿) [ERzeroToMany]`
꼴이다. 사람 말과 draw.io 원본 코드를 함께 둔다 — 원본이 있어야 그림에서
어느 선을 고쳐야 할지 다시 찾을 수 있다.

### `convert` — 논리 ERD → 물리 ERD

```bash
erdtool convert <file-or-dir> [--out dir] [--dictionary path] [--config path] [--recursive] [--dry-run]
```

표준용어사전(xlsx)을 써서 한글 논리명을 영문 물리명으로 바꿔
`X.drawio` → `X.physical.drawio`를 낸다.

- 원본 레이아웃(행 높이·키 열 너비·관계선)을 그대로 보존한다.
- 한글 논리명은 셀의 `logicalName` 속성에 남는다 — draw.io의
  «데이터 편집» 창에서 볼 수 있다.
- 조각을 하나도 못 찾으면 `[미변환]`으로 표시한다. 지어내지 않는다.
- 이미 변환된 `*.physical.drawio`는 폴더 처리 시 건너뛴다.

사전 경로는 `--dictionary`로 주거나 설정 파일에 적는다. 둘 다 없으면 대상
폴더에서 자동탐색한다.

### `build` — 데이터 설계서 → ERD

```bash
erdtool build <설계서.xlsx|csv> [--out FILE]
```

엑셀/CSV 설계서를 읽어 `.drawio`를 그린다. 페이지당 겹치지 않는 계층 배치,
행(컬럼)에 붙는 관계선과 카디널리티 방향, 키 열의 `PK`/`FK1` 표시까지
만든다. 입력 설계서를 출력으로 덮어쓰지 못하게 막는다.

설계서의 열 구성은 [`examples/README.md`](examples/README.md)에 있고,
[`examples/bookstore.csv`](examples/bookstore.csv)가 그 모양의 실물이다.

### `reverse` — DB 역공학 → ERD

```bash
erdtool reverse <접속이름|DSN|파일경로> [--out FILE] [--connections FILE]
```

**PostgreSQL · MySQL · SQLite** 세 DBMS에 직접 접속해 읽는다(DDL 덤프
파싱이 아니다). 스키마 하나가 페이지 하나가 되고, PK·FK·UNIQUE·NULL 여부와
카탈로그 원문 그대로의 타입을 가져온다.

- 출력 경로 기본값: SQLite는 DB 파일 옆에 `app.db` → `app.drawio`,
  서버는 작업 디렉터리에 **`<DB이름>.drawio`**. DSN의 경로에서 뽑은 DB
  이름이며 접속 설정의 섹션 이름이 아니다 — 섹션 `운영`이 `…/shopdb`를
  가리키면 `shopdb.drawio`가 나온다.
- **SQLite 파일 위에 `.drawio`를 덮어쓰지 못하게 막는다** — DB가 사라지면
  되돌릴 길이 없다.
- 에러 문구에서 비밀번호를 마스킹한다. 접속에 실패한 순간이 비밀번호가
  스크롤백에 박히는 순간이 되면 안 된다.
- 한국어 로케일 서버가 UTF-8이 아닌 바이트로 보내는 에러 문구를 CP949로
  읽어 준다.
- 스키마를 가로지르는 FK는 그리지 않고 «건너뛰었다»고 말한다.

접속 설정은 `erdtool.connections.example.yaml`을 복사해 쓴다.

### `annotate` — 검증 결과를 ERD에 되반영

```bash
erdtool annotate <file-or-dir> [--config path] [--recursive] [--dry-run] [--clean]
```

**사람이 그린 `.drawio`를 제자리에서 고쳐 쓰는 유일한 커맨드다.** 진단이 붙은
셀에 빨간 굵은 테두리를 입히고, 페이지마다 요약 박스를 단다. `--clean`이
역연산이다.

네 가지 계약을 지킨다:

1. **재실행 멱등** — 두 번 돌리면 바이트가 같다.
2. **`--clean` 왕복 안정** — `annotate` → `--clean`을 반복해도 매번 같은
   바이트가 나오고 `erdtool` 잔재가 0이 된다.
3. **사람이 옮긴 요약 박스의 자리와 레이어를 존중한다** — 매 실행 원래
   자리로 되돌리지 않는다.
4. **진단이 고쳐지면 표식을 회수한다** — 원래 스타일을 `erdtoolBaseStyle`
   속성에 기록해 두고 정확히 되돌린다.

압축된 페이지(실제 draw.io 저장 형식)도 다룬다.

`--clean` 뒤 파일은 **원본과 바이트가 같지는 않다.** Go의 `encoding/xml`이
self-closing 태그를 펼치기 때문이며, 그래서 계약 2는 「원본 바이트 동일」이
아니라 「정규화 왕복형과 동일」이다. 그림은 같고 잔재는 0이다.

### `watch` — 저장 감지 자동 실행

```bash
erdtool watch <dir> [--config path] [--recursive]
```

fsnotify로 `.drawio` 저장을 감지해 `generate`를 자동으로 돌린다. 같은
저장에서 겹쳐 들어오는 이벤트는 100ms 창으로 거른다.

### `version`

```bash
erdtool version
```

## 검증 규칙

`generate`의 검증 리포트와 `annotate`의 되반영이 **같은 규칙 열여섯**을 쓴다.
같은 파일을 둘에 먹이면 진단 건수가 같다 — 리포트를 보고 `annotate`를 돌린
사람이 「왜 숫자가 다르지」에서 멈추지 않는다(2026-09-22에 이었다).

| 규칙 | 심각도 | 내용 | 기본값 |
|---|---|---|---|
| `missing_primary_key` | warning | 테이블에 PK가 없음 | 켬 |
| `empty_table` | warning | 테이블에 컬럼이 없음 | 켬 |
| `missing_column_name` | warning | 정의 셀에 컬럼 이름이 없음 | 켬 |
| `missing_column_type` | warning | 정의 셀에 타입이 없음 | 켬 |
| `duplicate_column_name` | warning | 한 테이블에 같은 컬럼 이름이 둘 이상 | 켬 |
| `broken_reference` | warning | 관계선이 없는 테이블·행을 가리킴 | 켬 |
| `floating_relationship` | warning | 관계선이 아무것도 안 이었는데 그림에서는 테이블에 닿아 있음 | 켬 |
| `shape_violation` | 대상 밖 | 논리·물리설계 도형이 아니라 읽지 않음 | 켬 |
| `duplicate_table_name` | warning | 같은 이름 테이블이 여러 페이지에 있음 | 켬 |
| `attr_conflict` | warning | 같은 제약 이름이 한 컬럼에 둘인데 값이 다름 | 켬 |
| `unknown_attr` | warning | `erd_`로 시작하지만 읽지 않는 이름 | 켬 |
| `relationship_without_fk` | warning | 관계선은 있는데 어느 쪽이 외래키인지 표기가 없음 | 켬 |
| `non_ansi_type` | warning | ANSI SQL 타입이 아님 | 끔 |
| `naming_convention` | warning | 명명 규칙 위반 | 끔 |
| `ddl_syntax` | warning | 뽑은 DDL을 SQL 엔진이 거부함 | 켬 |
| `ddl_dryrun_unavailable` | warning | 시험 실행을 돌릴 SQL 엔진을 못 열었음 | 켬 |

### `shape_violation` — 「대상 밖」은 잘못이 아니다

이 도구는 **논리·물리설계만 읽는다.** 개념설계(Chen 표기) 도형과 설명용
낙서는 읽지 않는다. 그래도 리포트에 싣는 이유는, 사용자가 「이것도 정의서에
들어갔겠지」라고 믿은 채 넘어가지 않게 하기 위해서다.

그래서 표시가 다르다.

| | 결함 | 대상 밖 |
|---|---|---|
| 리포트 | 페이지·테이블별 목록 | 페이지 맨 뒤 「대상 아님」 절 |
| 줄머리 | **경고** | **대상 밖** |
| 건수 | 「진단 N건」 | 「읽지 않은 도형 N개」로 따로 센다 |
| `annotate` | 빨간 테두리 | **회색** 테두리 |

한 셀에 결함과 대상 밖이 함께 붙으면 빨강이 이긴다. 고쳐야 할 것이 회색에
묻히면 안 된다.

**페이지 성격은 판정하지 않는다.** 대상 여부는 도형마다 정해진다. 한
페이지에 테이블과 Chen 도형이 섞여 있어도 테이블은 정의서에 실리고, Chen
도형은 「대상 아님」에 뜬다. 관계형 도형이 없는 페이지는 뽑을 것이 없을
뿐이다.

### `floating_relationship` — 그림에는 이어져 보이는데 파일에는 연결이 없다

draw.io에서 관계선의 끝을 테이블 위에 놓아도 연결점이 안 잡히는 일이 있다.
그때 파일에 남는 것은 `source`·`target`이 아니라 끝점 좌표뿐이다. 화면에서는
두 테이블이 이어져 보이므로 그린 사람은 이었다고 믿는다.

이 검사가 없으면 그 관계선은 관계정의서에 안 실리고 진단도 안 뜬다. 없어진
사실 자체가 아무 데도 안 남는다.

**참고용 화살표와는 좌표로 가른다.** draw.io의 ER 도형 모음을 붙여 둔 설명
페이지에도 아무것도 잇지 않은 화살표가 있는데, 그것들은 테이블에서 멀리
떨어져 있다. 끝점이 테이블 사각형 안이나 그 언저리(10px)에 들어올 때만
진단을 낸다. 저장소 픽스처 전부에 돌려 위양성 0을 확인했다 — 지금은 열둘이고 그 안에
`source`/`target`이 없는 참고용 화살표가 서른 개 있는데, 진단이 나는 것은
그러라고 만든 픽스처 하나뿐이다.

### `ddl_syntax` — DDL을 실제로 실행해 본다

뽑아낸 `CREATE TABLE`을 **메모리 SQLite에 실행해 보고** 거부당하면 진단을 낸다.
서버도 파일도 새 의존성도 필요 없다. `--sql`을 안 줘도 검사는 실행된다.

이 검사만 잡는 것들이 있다. `NOT NULL`을 `NOTNULL`로 붙여 쓴 오타, 타입
괄호가 안 닫힌 것, `erd_check` 식의 괄호 오류, `erd_default`의 따옴표 오류.
앞 단계들은 전부 이것들을 통과시킨다.

**이 검사가 재는 것은 SQLite다.** 그래서 MySQL의 `enum(...)`·`set(...)`처럼
특정 DBMS 전용 타입은 여기서 걸린다 — 타깃 DBMS를 정하지 않은 지금은 ANSI가
기준이므로 걸리는 것이 맞지만, 「내 MySQL에서는 잘 도는데」가 답인 경우도
있다. 진단 문구가 무엇이 거부했는지를 밝히는 이유다.

앞 단계가 이미 진단을 낸 테이블은 건너뛴다. 같은 실수가 두 줄로 보고되면 사용자는
둘이 같은 것인지 다른 것인지 알 수 없다.

`duplicate_table_name`은 페이지를 가로지르는 사실이라 붙일 셀이 없다. 그래서
마크되지 않고 요약에만 실린다 — 없는 셀을 지어내지 않는다.

## 제약 적기 — `erd_*` 속성

ERD 그림에는 CHECK나 기본값을 적을 자리가 없다. 컬럼 한 줄은 셀이 둘뿐이다.
그래서 draw.io의 **데이터 편집**(`Ctrl+M`)으로 이름/값 쌍을 붙인다.

| 이름 | 값의 예 | 붙이는 곳 |
|---|---|---|
| `erd_check` | `LENGTH(CUST_NM) >= 2` | 컬럼, 테이블 |
| `erd_default` | `'미상'` | 컬럼만 |
| `erd_comment` | `고객명` | 컬럼, 테이블 |

붙일 대상은 **클릭 수**가 가른다. 테이블 제약은 한 번, 컬럼 제약은 두 번
누르면 잡히는 정의 셀이다.

읽는 쪽은 행·키 셀·정의 셀을 모두 그 컬럼의 것으로 받는다. 정의 셀로 좁히면
키 셀에 잘못 붙인 값이 아무 말 없이 사라지기 때문이다.

같은 이름이 한 컬럼에 둘인데 값이 다르면 **어느 쪽도 쓰지 않는다.** 조용히
하나를 고르면 사용자는 잃은 값을 끝까지 모른다. 그 자리는 검증 리포트의
`attr_conflict`, 정의서의 「충돌 — 값을 고르지 않음」, DDL의 주석 셋으로
드러난다.

이름을 잘못 치면 draw.io도 파서도 아무 말 하지 않으므로, `erd_`로 시작하는
모르는 이름은 `unknown_attr`로 올린다.

값은 손으로 쓴 SQL 식이라 **그대로 내보낸다.** 따옴표를 붙이거나 고쳐 쓰지
않는다 — 지레 감싸면 `now()`가 문자열이 되어 뜻이 바뀐다.

붙이는 순서와 함정은 [사용자설명서](docs/user-guide.md) 05절에 있다.

## 설정 파일

### `erdtool.yaml` — 동작 설정

명시하지 않으면 대상 폴더에서 자동탐색한다. `--config`를 주면 자동탐색을
**아예 하지 않는다** — 옆에 놓인 파일이 조용히 끼어들면 "왜 내가 준 설정이
안 먹지"가 되기 때문이다. 실제로 쓰인 설정 파일 경로는 화면에 보여 준다.
설정이 없는 것은 에러가 아니다.

```yaml
outputs:
  relation_doc: true          # 관계정의서를 낸다 (--relations와 같음)
  sql_ddl: true               # SQL DDL을 낸다 (--sql과 같음)

validation:
  ansi_sql_types: true        # 기본 끔
  naming_convention: true     # 기본 끔

page_as_domain: true          # 테이블 제목 옆에 페이지명을 «도메인»으로 표시
recursive: true               # 하위 폴더까지 읽는다 (--recursive와 같음)
output_dir: "docs/{basename}" # 산출물 위치. 입력 파일이 있는 폴더 기준
dialect: mysql               # 뽑을 SQL이 겨누는 DBMS (ansi/postgres/mysql/sqlite)
dictionary: "표준용어사전.xlsx"  # convert 전용. 작업 디렉터리 기준

pages:                        # 페이지별 재정의
  "논리 ERD":
    validation:
      ansi_sql_types: false
```

경로 필드의 상대 경로는 **설정 파일 위치가 아니라 실행한 작업 디렉터리**를
기준으로 푼다(`output_dir`만 입력 파일이 있는 폴더 기준이다).

### `erdtool.connections.yaml` — DB 접속 설정 (`reverse` 전용)

`erdtool.connections.example.yaml`을 복사해서 쓴다. 복사본은 `.gitignore`에
걸려 있다 — 비밀번호가 평문으로 들어가기 때문이다. 섹션 이름은 자유이고,
어느 DBMS인지는 섹션 이름이 아니라 DSN 스킴으로 정한다.

```yaml
connections:
  PostgreSQL:
    dsn: postgres://사용자@localhost:5432/DB이름?sslmode=disable
  MySQL:
    dsn: mysql://사용자:비밀번호@localhost:3306/DB이름
  SQLite:
    dsn: ./app.db
```

PostgreSQL은 비밀번호를 안 적어도 된다 — pgx가 `PGPASSWORD`와
`pgpass.conf`를 알아서 찾는다. DSN은 URL이므로 비밀번호의 특수문자는
퍼센트 인코딩한다(`@` → `%40`, `:` → `%3A`).

## 소스에서 빌드

Go 1.27.0 이상.

```bash
make build          # go build -o erdtool.exe ./cmd/erdtool
make test           # go test ./...
make cover          # CLI까지 포함한 커버리지 (bash scripts/coverage.sh)
make release VERSION=1.0.0   # dist/에 4개 플랫폼 바이너리. VERSION이 `erdtool version` 출력에 박힌다
```

`make cover`가 따로 있는 이유는 `scripts/coverage.sh` 머리에 있다. 서버는
필요 없다.

## 설계 원칙

이 저장소가 어겨서는 안 되는 규칙이다. 전부 실제로 버그를 낸 적이 있어서
남았다.

1. **스타일 문자열을 부분 문자열로 매칭하지 않는다.** `ParseStyle`로
   `key=value` 맵을 만든 뒤 정확히 비교한다. `"shape=table" in style`이
   `"shape=tableRow"`에도 걸리는 버그가 이 프로젝트가 존재하는 이유다.
2. **테이블→행→컬럼 3단 구조는 ID/parent 인덱스로만 탐색한다.** XML 문서
   순서에 의존하지 않는다.
3. **손으로 옮겨적은 예제 데이터를 믿지 않는다.** 실제 파일에서 프로그램으로
   추출해 왕복 검증한 뒤 쓴다.
4. **`exit 0`이 곧 정답이 아니다.** 실제 바이너리를 빌드해 돌리고 산출물을
   직접 확인한다.
5. **모르는 것을 지어내지 않는다.** 못 찾았으면 못 찾았다고, 건너뛰었으면
   건너뛰었다고 말한다.

## 만든 방식

코드와 문서는 Anthropic의 AI 코딩 도구 [Claude Code](https://claude.com/claude-code)와
함께 작성했다. 무엇을 만들지 정하고, 결과를 실물로 돌려 쓸 만한지 판정하는
것은 만든 사람이 했다.

작업 중에 남긴 설계 기록과 세션 기록은 공개하지 않는다. 코드 주석에 나오는
「원장」은 그 기록을 가리킨다.

## 이슈와 기여

**개인 프로젝트이며 이슈와 PR을 받지 않는다.** 답이 없는 채로 기다리게 하는
것보다 미리 말해 두는 편이 낫다고 보았다. 받아서 쓰는 것은 MIT 라이선스가
허락하는 만큼 자유롭다.

## 라이선스

MIT다. `LICENSE`에 전문이 있다.

> [!warning] 보증하지 않고, 책임지지 않는다
> 이 도구는 **있는 그대로** 제공된다. 어떤 보증도 없고, 이 도구를 써서 생긴
> 손해에 만든 사람은 책임을 지지 않는다. `LICENSE`의 마지막 두 문단이
> 그것이며, **구속력이 있는 것은 그 영문 원문이다** — 이 상자는 읽기 쉽게
> 옮긴 안내일 뿐이다.
>
> 특히 `annotate`는 **사람이 그린 `.drawio`를 제자리에서 고쳐 쓴다.**
> 처음 쓸 때는 `--dry-run`으로 무엇이 바뀔지 먼저 보고, 원본을 버전 관리
> 아래에 두거나 사본으로 돌려 보는 편이 낫다.

실행 파일에는 오픈소스 스물아홉이 들어 있고 그 고지는
[`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md)에 있다. 릴리스에도 함께
올라가므로 바이너리만 받은 사람도 조건을 볼 수 있다.

## 문서

**쓰는 사람을 위한 것**

- [`docs/user-guide.md`](docs/user-guide.md) — 사용자설명서. ERD를 어떻게
  그려야 도구가 읽는지, 검증 진단을 만나면 무엇을 하는지까지. 이 README는
  «어떤 명령이 있는가»이고, 그쪽은 «어떻게 쓰는가»다
- [`examples/README.md`](examples/README.md) — 예제 하나를 뜯어본다
- [`docs/drawio-er-palette.md`](docs/drawio-er-palette.md) — draw.io 「도형」
  패널 ER 카테고리 전수 (실측)
