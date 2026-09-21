# Go 빌드·런타임·배포 오버뷰

2026-09-21. 현재 구현과 앞으로 만들 배포 구조를 구분한다.

## 1. 소스 코드와 실행 파일

Go는 소스 코드를 실행 파일로 컴파일하는 언어다. 개발할 때는 Go 도구가 필요하지만, 실행할 때는 만들어진 바이너리를 실행한다.

```text
cmd/infra/main.go ─┐
                  ├─ 필요한 Go 패키지와 함께 컴파일 ─→ bin/infra
internal/... ─────┤
pkg/model/... ────┘

cmd/core/main.go ─┐
                 ├─ 필요한 Go 패키지와 함께 컴파일 ─→ bin/core
internal/... ────┤
pkg/model/... ───┘
```

`cmd` 아래 main package 하나가 프로그램의 진입점이다. `internal/operations` 등의 패키지는 별도 서버로 뜨지 않고 필요한 코드가 실행 파일에 포함된다. Go의 goroutine scheduler와 garbage collector 같은 런타임도 바이너리에 포함된다. 별도의 Go 서버나 JVM 같은 프로세스를 먼저 띄우는 방식이 아니다.

현재 프로젝트는 Go 표준 라이브러리만 사용한다. `go.mod`는 모듈 경로와 최소 Go 버전을 정의한다. 외부 Go 의존성이 없어서 지금은 `go.sum`이 없다. 의존성을 추가하면 해당 잠금 정보를 관리한다.

## 2. 개발 중 실행과 빌드

Go가 설치되어 있다면 repository root에서:

```sh
# 임시 실행 파일을 컴파일한 뒤 바로 실행한다.
go run ./cmd/infra -fixture examples/shared-storage.json impact storage-01

# 실행 파일을 bin/ 아래 만든다. 프로그램을 시작하는 명령은 아니다.
make build

# 이미 만들어진 실행 파일을 실행한다.
./bin/infra -fixture examples/shared-storage.json impact storage-01
```

`go run`은 인터프리터처럼 소스 한 줄씩 실행하는 것이 아니다. 개발 중 컴파일과 실행을 묶어주는 편의 명령이다. 배포에서는 보통 `go build`로 만든 바이너리를 사용한다.

`make check`는 포맷, race-enabled test, 정적 검사(`go vet`)를 실행한다. 테스트와 빌드는 구분하며 `make build`만으로 테스트 통과가 보장되지는 않는다.

## 3. 지금 실행하면 실제로 무슨 일이 일어나는가

### infra: 한 번 실행하고 종료하는 CLI

```text
infra 실행
  → fixture JSON 읽기
  → 이 프로세스의 메모리에 graph 생성
  → 공통 service로 조회
  → JSON 출력
  → 종료: 메모리와 graph 소멸
```

현재 CLI는 core 서버에 접속하지 않는다. 매 실행 데이터를 새로 읽고 resource ID도 새로 발급한다. 서로 다른 CLI 실행에서는 표시 이름과 kind/provider 필터를 사용한다.

### core: 여러 tool 요청을 받는 로컬 프로세스

```text
core 실행
  → fixture JSON 한 번 읽기
  → 메모리에 graph 유지
  → stdin JSON 요청 읽기
  → tool adapter → 공통 service
  → stdout JSON 응답
  → 다음 요청 대기
  → stdin EOF 또는 프로세스 종료: 상태 소멸
```

```sh
./bin/core -fixture examples/shared-storage.json
```

실행 후 아래 JSON을 입력하고 Enter를 누르면 도구 목록이 나온다.

```json
{"id":"1","method":"list_tools"}
```

같은 프로세스에 다음 요청을 입력할 수 있다.

```json
{"id":"2","method":"call_tool","name":"find_resources","arguments":{"kind":"nfs.Server"}}
```

반환받은 resource ID로 `get_impact` 등을 호출한다. 한 core 실행 내에서는 ID와 상태가 유지된다. `Ctrl-D`로 stdin을 닫으면 종료한다.

외부 agent 프로그램은 core를 subprocess로 띄워 stdin/stdout을 연결할 수 있다. LLM이 tool call을 생성하면 agent의 adapter가 이 프로세스에 전달하고, 결과를 다시 모델에게 돌려준다. core 자체는 LLM을 실행하거나 API key를 읽지 않는다.

현재 CLI와 core가 공유하는 것은 **서비스 코드**이며 **실행 중인 메모리**가 아니다. 둘을 동시에 띄우면 독립된 graph 두 개다. 지금은 network port, REST listener, PostgreSQL 연결, background discovery가 없다.

## 4. Mac에서 빌드한 프로그램과 Linux 프로그램

실행 파일에는 대상 운영체제와 CPU 아키텍처가 있다.

| 대상 | GOOS | GOARCH |
|---|---|---|
| Apple Silicon Mac | darwin | arm64 |
| Intel/AMD Linux 서버 | linux | amd64 |
| ARM Linux 서버 | linux | arm64 |

Go가 Mac에 설치되어 있으면 기본 `make build`는 해당 Mac용으로 만든다. Linux Go Docker 컨테이너 안에서 기본 빌드하면 Linux용으로 만들어지므로 그 바이너리를 Mac에서 직접 실행할 수는 없다.

현재 사용자 환경처럼 로컬 Go 없이 기존 Go Docker 이미지로 Apple Silicon Mac용 바이너리를 만들려면:

```sh
docker run --rm --network none \
  -e GOOS=darwin -e GOARCH=arm64 -e CGO_ENABLED=0 \
  -v "$PWD:/work" -w /work golang:1.25 make build

./bin/infra -fixture examples/shared-storage.json impact storage-01
```

테스트는 Go 컨테이너의 기본 Linux 환경에서 실행한다. cross-compilation 환경변수를 준 상태에서 Mac용 test binary를 Linux 컨테이너에서 실행하려고 하면 안 된다.

```sh
docker run --rm --network none \
  -v "$PWD:/work" -w /work golang:1.25 make check
```

나중에 Linux 서버용으로 만들 때는 예를 들어:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/core ./cmd/core
```

현재처럼 `CGO_ENABLED=0`으로 빌드 가능한 코드는 대상 서버에 Go를 설치할 필요가 없다. 향후 C 라이브러리를 사용하는 의존성을 도입하면 별도 runtime library와 cross-compilation 조건을 검토해야 한다. CA 인증서·설정 파일 같은 실행 자원도 필요에 따라 함께 배포한다.

## 5. 현재 CI와 앞으로의 릴리스

현재 GitHub Actions가 수행하는 작업:

```text
topic branch → PR
  → Go 1.25.x / stable 환경
  → 포맷 + race tests + vet
  → 두 바이너리 빌드
  → fixture CLI/tool smoke tests
  → 통과 후 main에 squash merge
```

**현재 CI는 테스트와 빌드 검증만 한다.** 바이너리 release 업로드, container image push, Helm 배포는 아직 하지 않는다. `bin/`은 gitignore에 있어 소스 저장소에 커밋하지 않는다.

배포 단계에서 추가할 흐름:

```text
검증된 main commit → 버전 태그
  → OS/CPU별 바이너리 빌드
  → checksum 및 release artifact
  → Linux용 container image
  → Helm chart가 해당 image version 참조
  → Kubernetes에 배포
```

Docker image는 실행 파일과 실행에 필요한 파일을 담는 배포 단위다. Go 소스나 컴파일러를 운영 image에 반드시 넣을 필요는 없다. Helm은 그 image를 어느 Deployment/DaemonSet에 몇 개 띄우고 어떤 설정·권한·볼륨을 줄지 정의한다.

## 6. 목표 중앙 런타임

다음 단계에서 원격 호출 adapter와 durable state를 갖추면:

```text
운영자 CLI / 외부 AI agent / UI
               │ 인증된 요청
               ▼
        중앙 core Deployment
        조회 / 재조정 / 진단
               │
           PostgreSQL
               ▲
      ┌────────┴─────────┐
Kubernetes provider   Linux node agents
   Deployment            DaemonSet

추가 provider: 스토리지 / 네트워크 / 가상화
추후 executor: 승인된 프로비저닝·운영 변경
```

중앙 core는 현재 상태와 작업을 PostgreSQL에 기록한다. provider는 native 시스템을 관측해 전송한다. Linux agent는 node별 한 프로세스이며 standalone Linux에서는 systemd service로 실행할 수 있게 한다. core 재시작 후에는 DB 상태와 provider 재동기화로 복구한다.

이때 CLI도 같은 중앙 service에 요청하도록 remote adapter를 붙인다. HTTP/gRPC 등의 transport는 이 단계에서 선택하며 OpenAPI나 MCP가 필수는 아니다. 조회·진단과 변경 실행의 권한은 분리한다.

현재 구현은 위 목표의 **모델·서비스 코드·로컬 호출 경로를 검증하는 첫 단계**다. 지금 프로그램을 실행한다고 Kubernetes/스토리지 서버에 접속하거나 인프라를 변경하지 않는다.
