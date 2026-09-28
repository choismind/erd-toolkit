"""scripts/gen_notices.py — THIRD-PARTY-NOTICES.md를 만든다.

왜 손으로 안 쓰나: 의존성은 바뀌는데 손으로 적은 고지는 안 고쳐진다. 그러면
배포하는 바이너리에 들어 있는 것과 고지가 어긋나고, 고지는 어긋나는 순간
제 구실을 잃는다.

무엇을 모으나: `go list -deps ./cmd/erdtool`이 내는 «실제로 링크되는» 패키지의
모듈만 본다. go.mod에 적혀 있어도 바이너리에 안 들어가는 것은 뺀다 —
빌드 도구나 테스트 전용 의존이 그렇다.

**네 플랫폼을 모두 보고 합친다.** `go list`는 기본으로 지금 도는 기계의
GOOS/GOARCH만 보는데, 의존성이 플랫폼마다 다르다 — Windows는
`mattn/go-isatty`와 `ncruces/go-strftime`을 링크하고 Linux는 그 대신
`google/uuid`를 링크한다. 한 플랫폼에서만 만들면 **나머지 세 바이너리의
고지가 빈다**, 그리고 그것은 다른 기계의 CI에서 «낡았다»로 터진다
(2026-09-22에 실제로 터졌다). 플랫폼 목록은 `scripts/build-release.sh`와
같아야 한다.

쓰는 법:

    python scripts/gen_notices.py            # THIRD-PARTY-NOTICES.md를 쓴다
    python scripts/gen_notices.py --check    # 낡았으면 종료 코드 1 (CI용)
"""

import io
import os
import re
import subprocess
import sys

OUT = "THIRD-PARTY-NOTICES.md"

# 라이선스 파일 이름. 앞에 있는 것이 이긴다.
NAMES = (
    "LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "LICENCE.md",
    "COPYING", "COPYING.md", "LICENSE-MIT", "LICENSE.BSD",
)


# 릴리스가 내는 플랫폼. scripts/build-release.sh와 같아야 한다.
PLATFORMS = (
    ("windows", "amd64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("linux", "amd64"),
)


def modules():
    """네 플랫폼의 바이너리에 들어가는 모듈을 합쳐 (경로, 버전, 디렉터리)로 낸다.

    합집합이라 «이 기계에서 안 쓰는» 모듈도 들어간다. 그래야 어느 기계에서
    돌려도 같은 파일이 나오고, 네 바이너리 중 하나만 받은 사람도 자기
    바이너리에 든 것의 고지를 다 본다.
    """
    seen = {}
    for goos, goarch in PLATFORMS:
        env = dict(os.environ, GOOS=goos, GOARCH=goarch)
        out = subprocess.run(
            ["go", "list", "-deps",
             "-f", "{{with .Module}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}",
             "./cmd/erdtool"],
            capture_output=True, text=True, encoding="utf-8", check=True, env=env,
        ).stdout
        for line in out.split("\n"):
            line = line.strip()
            if not line or line.startswith("erdtool|"):
                continue
            path, ver, d = line.split("|", 2)
            seen[path] = (ver, d)
    return sorted(seen.items())


def license_text(d):
    for n in NAMES:
        p = os.path.join(d, n)
        if os.path.exists(p):
            return n, io.open(p, encoding="utf-8", errors="replace").read().strip()
    return None, None


def spdx(text):
    """라이선스 전문에서 종류를 알아본다. 못 알아보면 그렇다고 적는다 —
    지어내면 고지가 틀린 말을 하게 된다."""
    t = text[:4000]
    if "Apache License" in t:
        return "Apache-2.0"
    if "Mozilla Public License" in t:
        return "MPL-2.0"
    if re.search(r"\bMIT License\b", t) or "Permission is hereby granted, free of charge" in t:
        return "MIT"
    if "Redistribution and use in source and binary forms" in t:
        return "BSD-3-Clause" if "Neither the name" in t else "BSD-2-Clause"
    if "GNU GENERAL PUBLIC" in t:
        return "GPL"
    if "GNU LESSER" in t:
        return "LGPL"
    return "확인 필요"


def build():
    """고지 본문과, 라이선스 파일을 못 찾은 모듈 목록과, 모듈 수를 낸다.

    모듈 수를 따로 돌려주는 이유: 예전에는 완성된 글에서 `\\n### `을 세었는데,
    **라이선스 전문 안에 `### Apache License ###`이라는 줄이 있는 모듈이 있어**
    한 개를 더 셌다. 그 수가 README와 작업 기록에 그대로 옮겨 적혀 있었다
    (2026-09-22에 발견).
    """
    mods = modules()
    b = []
    b.append("# 서드파티 고지\n")
    b.append("`erdtool` 실행 파일에는 아래 오픈소스가 들어 있다. 각 라이선스의\n"
             "고지 요구를 지키려고 전문을 그대로 싣는다.\n")
    b.append("**이 파일은 `python scripts/gen_notices.py`가 만든다.** 손으로 고치지\n"
             "말고 그 스크립트를 돌린다 — 의존성이 바뀌면 여기도 바뀌어야 한다.\n")
    b.append("## 한눈에 보기\n")
    b.append("| 모듈 | 버전 | 라이선스 |")
    b.append("|---|---|---|")
    missing = []
    for path, (ver, d) in mods:
        name, text = license_text(d)
        if text is None:
            missing.append(path)
            b.append("| `%s` | %s | **라이선스 파일을 못 찾음** |" % (path, ver))
        else:
            b.append("| `%s` | %s | %s |" % (path, ver, spdx(text)))
    b.append("")
    b.append("## 전문\n")
    for path, (ver, d) in mods:
        name, text = license_text(d)
        b.append("### %s %s\n" % (path, ver))
        if text is None:
            b.append("모듈에 라이선스 파일이 없다. 배포 전에 직접 확인한다.\n")
            continue
        b.append("```")
        b.append(text)
        b.append("```\n")
    return "\n".join(b) + "\n", missing, len(mods)


def main():
    check = "--check" in sys.argv
    text, missing, count = build()
    if check:
        old = io.open(OUT, encoding="utf-8").read() if os.path.exists(OUT) else ""
        if old != text:
            print("%s가 낡았다. `python scripts/gen_notices.py`를 돌려라." % OUT)
            return 1
        print("%s는 최신이다." % OUT)
        return 0
    io.open(OUT, "w", encoding="utf-8", newline="\n").write(text)
    print("%s를 썼다 (모듈 %d개)" % (OUT, count))
    if missing:
        print("라이선스 파일을 못 찾은 모듈:", ", ".join(missing))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
