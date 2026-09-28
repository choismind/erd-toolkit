# sakila.drawio에 «사람이 자주 저지르는 실수»를 하나씩 심는다.
# 검증이 무엇을 잡고 무엇을 놓치는지 보기 위한 것이다.
import re
import sys

src, dst = sys.argv[1], sys.argv[2]
s = open(src, encoding="utf-8").read()

# (설명, 바꾸기 전, 바꾼 뒤)
EDITS = [
    ("1. PK 표기를 지웠다 (country)", '<mxCell id="p0_t0_r0k" value="PK"', '<mxCell id="p0_t0_r0k" value=""'),
    ("2. 키 셀을 오타냈다 (actor: PK -> P K)", '<mxCell id="p0_t1_r0k" value="PK"', '<mxCell id="p0_t1_r0k" value="P K"'),
    ("3. 타입을 안 적었다 (category.category_id)", '<mxCell id="p0_t2_r0d" value="category_id tinyint unsigned NOT NULL"', '<mxCell id="p0_t2_r0d" value="category_id"'),
    ("4. NOT NULL을 붙여 썼다 (language.name)", 'value="name char(20) NOT NULL"', 'value="name char(20) NOTNULL"'),
    ("5. 컬럼 이름이 한 테이블에 둘이다 (city.city_id 중복)", 'value="city varchar(50) NOT NULL"', 'value="city_id varchar(50) NOT NULL"'),
    ("6. 정의 셀을 비웠다 (address.district)", 'value="district varchar(20) NOT NULL"', 'value=""'),
    ("7. 테이블 이름이 겹친다 (film_text -> film)", '<mxCell id="p0_t15" value="film_text"', '<mxCell id="p0_t15" value="film"'),
    ("8. 관계선이 없는 셀을 가리킨다", 'source="p0_t4_r0"', 'source="p0_t4_r0_지워진행"'),
    ("9. ER 도형이 아닌 것을 섞어 그렸다", '<mxCell id="p0_t15_r0k"', '<mxCell id="p0_t15_r0k_note" style="ellipse;whiteSpace=wrap;html=1;" parent="1" vertex="1"/><mxCell id="p0_t15_r0k"'),
    ("10. 관계선은 그렸는데 FK 표기를 안 했다 (rental.staff_id)", '<mxCell id="p0_t13_r5k" value="FK3"', '<mxCell id="p0_t13_r5k" value=""'),
]

for label, before, after in EDITS:
    if s.count(before) != 1:
        sys.exit("«%s» — 바꿀 자리가 %d곳이다 (1곳이어야 한다)" % (label, s.count(before)))
    s = s.replace(before, after, 1)
    print("심었다:", label)

open(dst, "w", encoding="utf-8").write(s)
