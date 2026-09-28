플랫폼에 맞는 파일을 받아 바로 실행한다. 설치할 것이 없다.

| 파일 | 어디서 |
|---|---|
| `erdtool-windows-amd64.exe` | Windows |
| `erdtool-darwin-arm64` | macOS (Apple Silicon) |
| `erdtool-darwin-amd64` | macOS (Intel) |
| `erdtool-linux-amd64` | Linux |

`SHA256SUMS`로 받은 파일이 온전한지 확인할 수 있다.

**첫 실행에 쓸 예제는 `erdtool-examples.zip`에 있다.** 풀고 나서 한 줄이다.

```bash
erdtool generate examples/bookstore.drawio --relations --sql
```

**Windows에서는 서명이 없어 SmartScreen이 막을 수 있다.** 「추가 정보」를
누르고 「실행」을 고른다.

macOS와 Linux에서는 받은 뒤 실행 권한을 준다.

```bash
chmod +x erdtool-darwin-arm64
./erdtool-darwin-arm64 version
```

사용법은 [README](../README.md)와 [사용자설명서](../docs/user-guide.md)에 있다.
그림을 그리거나 산출물을 열려면 [draw.io](https://www.drawio.com/)가 필요하다.

라이선스는 MIT다. 실행 파일에 들어간 오픈소스의 고지는 함께 올린
`THIRD-PARTY-NOTICES.md`에 있다.

**보증하지 않고, 책임지지 않는다.** 이 도구는 있는 그대로 제공되며, 써서
생긴 손해에 만든 사람은 책임을 지지 않는다. 구속력이 있는 것은 함께 올린
`LICENSE`의 영문 원문이고, 이 문단은 읽기 쉽게 옮긴 안내다.
