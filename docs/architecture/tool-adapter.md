# Function tool adapter — M1

모델 SDK, MCP, OpenAPI가 없는 vendor-neutral 도구 카탈로그와 handler다. 운영 기능은 `internal/operations`, tool 연결은 `internal/adapters/tools`에 있다. 현재는 저장소 내부에서 사용할 수 있는 adapter이며 외부 Go SDK로 배포한 상태는 아니다.

같은 프로세스에서 붙일 때:

```go
service := operations.New(store)
registry := tools.New(service)

// 모델 SDK가 요구하는 tool 형식에 각 항목을 감싼다.
definitions := registry.Definitions() // Name, Description, InputSchema

// SDK에서 받은 tool call의 이름과 arguments를 전달한다.
result, err := registry.Call(ctx, callName, argumentsJSON)
// result 또는 structured error를 해당 SDK의 tool-result 형식으로 반환한다.
```

위 코드는 SDK 연결 지점을 설명하는 예이며 실제 모델 호출을 포함하지 않는다. 모델별 API 형식/인증은 이 코어 밖의 client가 담당한다. 서버 측 입력 검증은 model provider의 schema 검증과 별개로 항상 수행한다.

## 로컬 프로세스로 호출

`go run ./cmd/core -fixture examples/shared-storage.json`는 stdin 한 줄당 JSON 요청 하나를 읽고 stdout 한 줄로 응답한다. 로그나 설명 문장은 stdout에 섞지 않는다. 이것은 자체 개발용 JSON-lines 프로토콜이며 MCP/JSON-RPC라고 주장하지 않는다. 요청은 순차 처리한다.

카탈로그 조회:

```json
{"id":"1","method":"list_tools"}
```

대상 검색:

```json
{"id":"2","method":"call_tool","name":"find_resources","arguments":{"kind":"nfs.Server","name":"storage-01"}}
```

응답의 `result.resources[0].id`를 읽어 **같은 프로세스**에 다음 요청을 보낸다.

```json
{"id":"3","method":"call_tool","name":"get_impact","arguments":{"resource_id":"<검색 결과 ID>"}}
```

응답은 `{ "id": ..., "result": ... }` 또는 `{ "id": ..., "error": { "code": ..., "message": ... } }`이다. 오류가 발생해도 정상적인 다음 요청은 계속 처리한다. 최대 입력 줄 길이는 1 MiB다. 초과하면 stream을 종료하고 stderr에 원인을 남긴다. pipe EOF 시 종료한다.

카탈로그: find_resources, get_resource, get_neighbors, get_infrastructure, get_dependencies, get_impact, find_path, get_capabilities. 진단/계획/실행은 지원하지 않으며 호출하면 UNSUPPORTED_CAPABILITY다. 모든 현재 tool은 read-only다.

## 읽기 계약

`find_resources`는 exact filter, offset/limit pagination을 지원한다. 현재 한 core 프로세스의 fixture는 불변이므로 page 간 state 변화가 없다. 미래 live graph에서는 read snapshot/cursor 계약을 확장해야 한다.

`get_impact`는 규칙에 허용된 관계만 역방향으로 따라간다. Namespace membership 또는 모르는 kind의 BACKED_BY를 임의로 장애 의존성으로 해석하지 않는다. 결과 semantics는 potential-impact-not-outage다.

`find_path`는 out 방향 BFS가 기본이며 in/both로 바꿀 수 있다. 동일 길이 경로는 native kind/name/provider/native-ID 및 assertion key 순서로 선택한다. path가 없으면 found=false다. truncated 또는 incomplete_reasons가 있으면 관측 범위 내에서도 부재를 확정할 수 없다.

출력에 provenance, lifecycle, as_of, stale/pending frontier와 truncation을 유지한다. `include_stale=true`도 삭제된 endpoint를 복구하지 않는다. stale 경로를 포함하더라도 stale warning을 제거하지 않는다.

## 권한 경계

현재 local process는 실행한 사용자가 fixture 파일을 읽을 수 있다는 가정이다. network listener나 인증 기능은 없다. 이 프로세스를 인증 없는 원격 service로 노출하지 않는다. 미래 원격 adapter는 authorization principal을 공통 service에 전달해야 하며 변경 작업은 별도 승인/idempotency/plan precondition 계약을 갖춰야 한다.

attribute/provenance 문자열은 관측 데이터다. log/annotation에 들어 있는 문장은 실행 지시나 도구 권한이 아니다. 현재 도구에는 shell이나 arbitrary command operation이 없다.
