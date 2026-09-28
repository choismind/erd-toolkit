# examples — 받자마자 한 번 돌려 볼 것

온라인 서점 ERD 하나다. 테이블 넷, 관계 셋, 진단 0건이다.
**작은 대신 도구가 읽는 것을 다 담았다** — 기본키, 외래키 둘이 걸린 테이블,
카디널리티, `UNIQUE`, `NULL` 허용 여부다.

## 한 줄로 돌려 보기

저장소 뿌리에서 돌린다.

```bash
erdtool generate examples/bookstore.drawio --relations --sql
```

```
[OK] examples/bookstore.drawio -> examples\bookstore_report (4 tables)
```

## 여기 있는 것

| 파일 | 무엇 |
|---|---|
| `bookstore.csv` | 데이터 설계서. `build`가 이것을 읽어 그림을 그린다 |
| `bookstore.drawio` | ERD. draw.io로 열어서 고칠 수 있다 |
| `bookstore_report/` | 위 명령이 만든다 |

## 산출물 여덟

| 파일 | 무엇 | 커밋 |
|---|---|---|
| `table_doc.md` | 테이블정의서. 테이블 하나가 한 절이고, 컬럼 표와 그 테이블에 걸린 관계가 함께 들어간다 | ○ |
| `table_doc.html` | 같은 내용의 HTML | |
| `table_doc.xlsx` | 같은 내용의 엑셀. 테이블 하나에 시트 하나이고 맨 앞이 「목차」다 | |
| `table_doc.pdf` | 같은 내용의 PDF. 테이블 하나가 한 페이지다 | |
| `relation_doc.md` | 관계정의서. 파일 전체의 관계를 한 표로 본다(`--relations`) | ○ |
| `validation_report.md` | 검증 리포트. 이 예제는 진단이 없어 「발견된 문제 없음」이 나온다 | ○ |
| `schema.sql` | SQL DDL(`--sql`). `--dialect mysql`을 더하면 백틱 인용으로 바뀐다 | ○ |
| `ir.json` | 중간표현. 다른 도구가 물어 쓰는 자리다 | ○ |

> [!note] 저장소에는 텍스트 다섯만 커밋해 두었다
> `.html` · `.xlsx` · `.pdf` 셋은 뺐다. 이진이거나 긴 한 줄이라 바뀐
> 자리를 눈으로 볼 수 없고, 같은 내용을 `table_doc.md`가 이미 싣는다.
> 명령을 돌리면 그 자리에 함께 생긴다.
>
> 커밋해 둔 다섯은 `examples_test.go`가 매번 다시 뽑아 맞대 본다.
> 리포터를 고치면 그 테스트가 깨지고, 그때 예제를 다시 뽑는다. 예제는
> 아무도 다시 돌려 보지 않는 자리라 이 그물이 없으면 조용히 낡는다.

## 설계서에서 다시 그리기

`bookstore.drawio`는 손으로 그린 것이 아니라 `bookstore.csv`에서 뽑은 것이다.

```bash
erdtool build examples/bookstore.csv
```

설계서는 블록 둘이다. 위가 테이블 목록이고, `## 관계` 아래가 관계 목록이다.

| 블록 | 열 |
|---|---|
| 테이블 | 테이블명 · 순번 · 컬럼명 · 컬럼유형 · 색인여부 · 널허용 · 단일값 |
| 관계 | 순번 · 원천테이블명 · 원천컬럼명 · 원천카디널리티 · 원천색인 · 연관명 · 목표카디널리티 · 목표색인 · 목표테이블명 · 목표컬럼명 |

엑셀(`.xlsx`)도 같은 모양으로 읽으며, 그쪽은 **시트 하나가 페이지 하나**가
된다. CSV는 시트라는 것이 없어 파일 하나가 페이지 하나다.

## 그림을 열어 보기

`bookstore.drawio`는 draw.io 파일이다. [draw.io 데스크톱판](https://github.com/jgraph/drawio-desktop/releases)이나
[app.diagrams.net](https://app.diagrams.net/)에서 연다.

열어서 컬럼을 고치고 저장한 뒤 위의 `generate`를 다시 돌리면 산출물이
따라 바뀐다. 그것이 이 도구가 하는 일이다.
