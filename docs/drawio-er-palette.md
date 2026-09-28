# draw.io "Entity Relation" 도형 라이브러리 전수 (v31.1.5 실측)

Phase 1b(Chen 표기법 파싱) 설계의 «사실 근거» 문서다. 총칭이 아니라 실제
스타일 문자열을 담는다 — 작업 기록이 예전에 "스펙 문서의 허용 도형 세트 절
참고"라고 가리켰지만 그 절에는 도형 목록이 없어 헛걸음을 만들었다.

## 추출 방법 (재현 가능)

설치된 draw.io Desktop의 앱 리소스에서 직접 뽑았다. 문서를 옮겨적은 것이
아니라 프로그램이 읽는 원본이다(프로젝트 원칙 3번).

```
C:\Program Files\draw.io\resources\app.asar   (v31.1.5, 오프셋 ~47.7MB 부근)
```

라이브러리 항목을 등록하는 호출은 **네 가지**다 — `this.addEntry(태그, ...)`,
`this.addDataEntry(태그, w, h, 제목, 압축XML)`, `createVertexTemplateEntry(...,
태그)`, `createEdgeTemplateEntry(..., 태그)`. 이 중 태그에 `er entity relation`이
들어간 것을 골라냈다.

라이브러리 항목의 태그는 예외 없이 `db database schema er entity relation table `로
시작하고 그 뒤에 구분어가 붙는다(`... table attribute key chen`처럼). 그중
`chen`이 붙은 것이 Chen 표기법 계열이다. draw.io 자신이 검색·분류에 쓰는
태그이므로 이 기준이 가장 믿을 만하다.

**실측 개수** (2026-09-03 재측정): 태그가 붙은 ER 항목 **50개**.

| 등록 호출 | 개수 | 무엇 |
|---|---|---|
| `addEntry` | 16 | A계열 5(List·List Item 1~3·Entity) + Hierarchy + **Chen 관계선 10** |
| `createVertexTemplateEntry` | 13 | **Chen 도형 10** + Entity·Cloud·Note |
| `createEdgeTemplateEntry` | 16 | C계열 카디널리티 엣지 전부 |
| `addDataEntry` | 5 | Table 1·2, Table Row 1~3 — `shape=table` 원형 |

그중 `chen` 태그가 붙은 것이 **20개**(도형 10 + 엣지 10). Chen 관계선은
`createEdgeTemplateEntry`가 아니라 `addEntry`로 등록된다 — 라벨 달린 셀을
직접 만들어야 하기 때문이다.

> [!warning] 이 문서가 한동안 "45개"라고 적어 둔 이유
> 첫 추출이 `addEntry`와 `create*TemplateEntry`만 읽고 **`addDataEntry`를
> 빼먹었다**(16 + 29 = 45). 빠진 5개가 하필 Table 1·2와 Table Row 1~3 —
> erdtool이 다루는 관계형 테이블의 원형이다. 당시의 작업 기록과 Phase 1a
> 설계서가 적어 둔 **"총 50개"가 맞았고**, 그것을 틀렸다고 적은 이쪽이
> 틀렸었다. `chen` 20개와 C계열 16개는 영향 없다.

## 50개 중 코드로 식별되는 것은 몇 개인가 (2026-09-03 실측)

**스타일 문자열이 서로 다른 것은 50개 중 42개다.** 8개는 다른 항목과 글자
하나까지 같아서 스타일만으로는 어느 항목인지 갈라낼 수 없다:

- Chen 관계선 10개 → 스타일 **3종**뿐(`endArrow=none;...` 4개,
  `...dashed=1;dashPattern=1 2;` 3개, `shape=link;...` 3개). 카디널리티가
  스타일이 아니라 **라벨 텍스트**(`1`/`N`/`M`)에 있기 때문이다.
- Table Row 2 · Table Row 3 → 바깥 셀 스타일이 완전 동일.

스타일이 고유하다고 «ER 항목»이라 단정되는 것도 아니다. `rounded=1`(둥근
사각형), `ellipse`, `shape=rhombus`는 아무 다이어그램에나 있는 도형이다.
**ER 전용 토큰**(`edgeStyle=entityRelationEdgeStyle`, `shape=table` +
`childLayout=tableLayout`, `mxgraph.er.anchor`)을 가진 것만 세면 22개다.

erdtool의 `ClassifyShape`(`internal/drawio/classify_shapes.go`)에 항목의
최상위 셀을 그대로 넣으면 이렇게 갈린다:

| 판정 | 개수 | 항목 |
|---|---|---|
| `Parsed` | **23** | C계열 엣지 16 + Table 1·2 + Table Row 1~3 + List Item 1·2(`shape=partialRectangle`) |
| `Ignored` | **1** | Note |
| `Violation` | **26** | Chen 20 + List · List Item 3(`line`) · Entity(swimlane) + Entity(민 사각형) + Cloud + Hierarchy |

즉 **라이브러리 항목 50개 중 erdtool이 구조로 읽는 것은 23개**이고, 나머지는
읽지 않는다는 판정이 이미 코드에 박혀 있다.

## B 계열 — Chen 표기법 도형 10개

| 이름 | 스타일 | 고유 식별 가능? |
|---|---|---|
| Entity (Rounded) | `rounded=1;arcSize=10;whiteSpace=wrap;html=1;align=center;` | **불가** |
| Weak Entity | `shape=ext;margin=3;double=1;whiteSpace=wrap;html=1;align=center;` | 가능 (`shape=ext`+`double=1`) |
| Attribute | `ellipse;whiteSpace=wrap;html=1;align=center;` | **불가** |
| Key Attribute | `ellipse;...;fontStyle=4;` | **불가** (굵게일 뿐) |
| Weak Key Attribute | `ellipse;...;fontStyle=20;` | **불가** |
| Derived Attribute | `ellipse;...;dashed=1;` | **불가** |
| Multivalue Attribute | `ellipse;shape=doubleEllipse;margin=3;...` | 가능 (`shape=doubleEllipse`) |
| Associative Entity | `shape=associativeEntity;whiteSpace=wrap;html=1;align=center;` | 가능 |
| Relationship | `shape=rhombus;perimeter=rhombusPerimeter;...` | 가능 (`shape=rhombus`) |
| Identifying Relationship | `shape=rhombus;double=1;perimeter=rhombusPerimeter;...` | 가능 |

## B 계열 — Chen 관계선 10개

**카디널리티가 화살표가 아니라 «엣지 라벨»(`1`/`N`/`M`)로 표현된다.**
Phase 1a의 `entityRelationEdgeStyle` + `endArrow=ERone` 방식과 완전히 다르다.

| 이름 | 스타일 | 라벨 |
|---|---|---|
| Untitled Relation | `endArrow=none;html=1;rounded=0;` | 없음 |
| Mandatory Participation (0:1) | `endArrow=none;html=1;rounded=0;` | `1` |
| Mandatory Participation (0:N) | `endArrow=none;html=1;rounded=0;` | `N` |
| Mandatory Participation (M:N) | `endArrow=none;html=1;rounded=0;` | `M`, `N` |
| Optional Participation (0:1) | `endArrow=none;html=1;rounded=0;dashed=1;dashPattern=1 2;` | `1` |
| Optional Participation (0:N) | `endArrow=none;html=1;rounded=0;dashed=1;dashPattern=1 2;` | `N` |
| Optional Participation (M:N) | `endArrow=none;html=1;rounded=0;dashed=1;dashPattern=1 2;` | `M`, `N` |
| Recursive Relationship (0:1) | `shape=link;html=1;rounded=0;` | `1` |
| Recursive Relationship (0:N) | `shape=link;html=1;rounded=0;` | `N` |
| Recursive Relationship (M:N) | `shape=link;html=1;rounded=0;` | `M`, `N` |

## 나머지 계열 (Phase 1a가 이미 처리)

- **A(테이블)**: `addEntry` 5종 — List / List Item 1~3 / Entity(swimlane).
  여기에 `addDataEntry` 5종이 더 있다 — **Table 1·2**(`shape=table;startSize=30;
  childLayout=tableLayout;...`), **Table Row 1~3**(제목 없는 `shape=table` +
  안쪽 행). 압축 XML로 등록되어 있어 스타일 문자열 grep에 안 잡힌다.
  실제 ERD 테이블은 `shape=table` + `shape=tableRow` + `shape=partialRectangle`
  3단 구조(스펙 문서 "허용 도형 세트" 참고).
- **C(카디널리티 엣지)**: `edgeStyle=entityRelationEdgeStyle` 16종 —
  아래 "C 계열 전수" 절에 목록이 있다.
- **D(기타)**: Entity(`whiteSpace=wrap;html=1;align=center;` — 평범한 사각형,
  chen 태그 **없음**), Cloud(`ellipse;shape=cloud;`), Note(`shape=note;size=20;`),
  Hierarchy(chen 태그 없음 — 스펙 문서에 별도 분석 있음).

## C 계열 전수 — 카디널리티 엣지 16종 (2026-08-27 실측)

Phase 2b(DB 역공학) 설계 중에 뽑았다. 추출 위치는 `app.asar` 오프셋
**~47,691,500** 부근의 `createEdgeTemplateEntry` 연속 블록이다(v31.1.5).
공통 접두는 전부 `edgeStyle=entityRelationEdgeStyle;fontSize=12;html=1;`이다.

**이름은 draw.io 자신이 붙인 것**이며, 의미 판정의 근거가 된다.

| # | 화살표 | draw.io가 붙인 이름 |
|---|---|---|
| 1 | `endArrow=ERone;endFill=1` | 1 |
| 2 | `endArrow=ERmandOne` | 1 Mandatory |
| 3 | `endArrow=ERzeroToOne;endFill=1` | 0 to 1 |
| 4 | `endArrow=ERmany` | Many |
| 5 | `endArrow=ERzeroToMany;endFill=1` | 0 to Many Optional |
| 6 | `endArrow=ERoneToMany` | 1 to Many |
| 7 | `endArrow=ERmandOne;startArrow=ERmandOne` | 1 to 1 |
| 8 | `endArrow=ERmany;startArrow=ERmany` | Many to Many |
| 9 | `endArrow=ERzeroToMany;startArrow=ERzeroToOne` | 1 Optional to Many Optional |
| 10 | `endArrow=ERzeroToMany;startArrow=ERmandOne` | 1 Mandatory to Many Optional |
| 11 | `endArrow=ERzeroToOne;startArrow=ERmandOne` | 1 Mandatory to 1 Optional |
| 12 | `endArrow=ERoneToMany;startArrow=ERmandOne` | 1 Mandatory to Many Mandatory |
| 13 | `endArrow=ERoneToMany;startArrow=ERzeroToOne` | 1 Optional to Many Mandatory |
| 14 | `endArrow=ERoneToMany;startArrow=ERzeroToMany` | Many Optional to Many Mandatory |
| 15 | `endArrow=ERoneToMany;startArrow=ERoneToMany` | Many Mandatory to Many Mandatory |
| 16 | `endArrow=ERzeroToMany;endFill=1;startArrow=ERzeroToMany` | Many Optional to Many Optional |

**16종은 새 어휘가 아니다.** 전부 화살표 코드 6종(`ERone`/`ERmandOne`/
`ERzeroToOne`/`ERzeroToMany`/`ERoneToMany`/`ERmany`)을 양 끝에 조합한
프리셋이다. `genbuild.RelationDef`는 양 끝을 독립적으로 지정하므로 6×6=36
조합을 낼 수 있고 이 16종은 그 부분집합이다.

**이름이 알려주는 의미 — Phase 2b가 이것에 의존한다.**
`ERzeroToMany`는 "Many **Optional**", `ERoneToMany`는 "Many **Mandatory**"다.
즉 둘의 차이는 개수가 아니라 **참여 필수성**이며, `ERoneToMany`는 「부모에게
자식이 반드시 하나 이상 있다」는 주장이다. 표준 SQL에는 그것을 선언할 문법이
없으므로 스키마만 읽는 역공학은 그 코드를 쓸 수 없다(Phase 2b 스펙 "카디널리티"
절 참고).

## Phase 1b 설계에 직결되는 사실

1. **Chen 도형 10개 중 5개는 스타일만으로 식별할 수 없다.** Entity(Rounded)는
   그냥 둥근 사각형이고, Attribute 4종은 전부 `ellipse`에 굵게/점선/폰트만
   다르다. 아무 다이어그램에나 있는 평범한 도형과 구분이 안 된다 —
   **스타일 매칭만으로 Chen 파서를 만들 수 없다.** 연결 구조(타원이 마름모에
   붙어 있다 등)를 함께 봐야 한다.
2. **`ellipse`와 `rounded=1`은 값 없는 토큰이다.** `ParseStyle`은 이런 토큰을
   `key: ""`로 담으므로(`internal/drawio/style.go`), 판정은 «키 존재 여부»로
   해야 한다 — `style["ellipse"] == "ellipse"` 같은 비교는 성립하지 않는다.
   프로젝트 원칙 1번(부분 문자열 금지, 정확 비교)은 그대로 유효하다.
3. **Chen 관계선에는 고유 스타일이 사실상 없다.** `endArrow=none`은 아무
   선에나 붙는다. 카디널리티는 엣지의 «라벨 텍스트»에서 읽어야 한다.
4. 위 세 가지 때문에 **"이 페이지가 Chen 다이어그램인가"를 도형 하나로 판정할
   수 없다.** 페이지 전체의 구조를 보고 판정해야 하며, 이것이 작업 기록의
   **I3** 항목(현재의 conceptual 판별 기준이 "파서가 모르는 도형이 많다"에
   불과하다는 문제)와 정면으로 얽힌다.
