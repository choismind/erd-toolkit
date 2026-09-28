# 관계정의서

| 페이지 | 원천 테이블 | 원천 컬럼 | 카디널리티(원천) | 카디널리티(목표) | 목표 컬럼 | 목표 테이블 |
|---|---|---|---|---|---|---|
| bookstore | MEMBER | MBR_NO | 1 (정확히 하나) [ERone] | 0..N (없거나 여럿) [ERzeroToMany] | MBR_NO | ORD |
| bookstore | ORD | ORD_NO | 1 (정확히 하나) [ERone] | 0..N (없거나 여럿) [ERzeroToMany] | ORD_NO | ORD_ITEM |
| bookstore | BOOK | BOOK_NO | 1 (정확히 하나) [ERone] | 0..N (없거나 여럿) [ERzeroToMany] | BOOK_NO | ORD_ITEM |
