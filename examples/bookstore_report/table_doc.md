# 테이블정의서

- **원본**: bookstore.drawio
- **기준 시각**: 2026-09-22 10:08
- **타깃 DBMS**: ANSI (지정 안 함)
- **생성 도구**: erdtool 0.1.0-dev
- **테이블 수**: 4개

## 테이블 일람

| No | 테이블명 | 설명 | 컬럼 수 | 페이지 |
|---|---|---|---|---|
| 1 | MEMBER |  | 4 | bookstore |
| 2 | BOOK |  | 4 | bookstore |
| 3 | ORD |  | 3 | bookstore |
| 4 | ORD_ITEM |  | 4 | bookstore |

## 1. MEMBER

페이지: bookstore

| No | 컬럼명 | 데이터 타입 | PK | FK | 참조 | NULL 허용 | 기본값 | 제약 | 설명 |
|---|---|---|---|---|---|---|---|---|---|
| 1 | MBR_NO | int | ● |  |  | N |  |  |  |
| 2 | MBR_NM | varchar(50) |  |  |  | N |  |  |  |
| 3 | EMAIL | varchar(100) |  |  |  | Y |  | UNIQUE |  |
| 4 | JOIN_YMD | date |  |  |  | N |  |  |  |

**관계**

| No | 방향 | 상대 테이블 | 이 테이블 컬럼 | 상대 컬럼 | 카디널리티(이쪽) | 카디널리티(상대) |
|---|---|---|---|---|---|---|
| 1 | ← 참조됨 | ORD | MBR_NO | MBR_NO | 1 (정확히 하나) [ERone] | 0..N (없거나 여럿) [ERzeroToMany] |

## 2. BOOK

페이지: bookstore

| No | 컬럼명 | 데이터 타입 | PK | FK | 참조 | NULL 허용 | 기본값 | 제약 | 설명 |
|---|---|---|---|---|---|---|---|---|---|
| 1 | BOOK_NO | int | ● |  |  | N |  |  |  |
| 2 | TITLE | varchar(200) |  |  |  | N |  |  |  |
| 3 | AUTHOR | varchar(100) |  |  |  | Y |  |  |  |
| 4 | PRICE | int |  |  |  | N |  |  |  |

**관계**

| No | 방향 | 상대 테이블 | 이 테이블 컬럼 | 상대 컬럼 | 카디널리티(이쪽) | 카디널리티(상대) |
|---|---|---|---|---|---|---|
| 1 | ← 참조됨 | ORD_ITEM | BOOK_NO | BOOK_NO | 1 (정확히 하나) [ERone] | 0..N (없거나 여럿) [ERzeroToMany] |

## 3. ORD

페이지: bookstore

| No | 컬럼명 | 데이터 타입 | PK | FK | 참조 | NULL 허용 | 기본값 | 제약 | 설명 |
|---|---|---|---|---|---|---|---|---|---|
| 1 | ORD_NO | int | ● |  |  | N |  |  |  |
| 2 | MBR_NO | int |  | FK1 | MEMBER.MBR_NO | N |  |  |  |
| 3 | ORD_YMD | date |  |  |  | N |  |  |  |

**관계**

| No | 방향 | 상대 테이블 | 이 테이블 컬럼 | 상대 컬럼 | 카디널리티(이쪽) | 카디널리티(상대) |
|---|---|---|---|---|---|---|
| 1 | → 참조함 | MEMBER | MBR_NO | MBR_NO | 0..N (없거나 여럿) [ERzeroToMany] | 1 (정확히 하나) [ERone] |
| 2 | ← 참조됨 | ORD_ITEM | ORD_NO | ORD_NO | 1 (정확히 하나) [ERone] | 0..N (없거나 여럿) [ERzeroToMany] |

## 4. ORD_ITEM

페이지: bookstore

| No | 컬럼명 | 데이터 타입 | PK | FK | 참조 | NULL 허용 | 기본값 | 제약 | 설명 |
|---|---|---|---|---|---|---|---|---|---|
| 1 | ORD_ITEM_NO | int | ● |  |  | N |  |  |  |
| 2 | ORD_NO | int |  | FK1 | ORD.ORD_NO | N |  |  |  |
| 3 | BOOK_NO | int |  | FK2 | BOOK.BOOK_NO | N |  |  |  |
| 4 | QTY | int |  |  |  | N |  |  |  |

**관계**

| No | 방향 | 상대 테이블 | 이 테이블 컬럼 | 상대 컬럼 | 카디널리티(이쪽) | 카디널리티(상대) |
|---|---|---|---|---|---|---|
| 1 | → 참조함 | ORD | ORD_NO | ORD_NO | 0..N (없거나 여럿) [ERzeroToMany] | 1 (정확히 하나) [ERone] |
| 2 | → 참조함 | BOOK | BOOK_NO | BOOK_NO | 0..N (없거나 여럿) [ERzeroToMany] | 1 (정확히 하나) [ERone] |
