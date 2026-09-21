# Product direction — infrastructure operations platform

Status: product direction clarified; shared-service/tool-adapter foundation implementation started. 2026-09-21.

## 제품 정의

사람과 외부 AI 에이전트가 공통 API를 통해 실제 인프라를 이해하고, 네트워크·스토리지 장애를 진단하며, 프로비저닝과 운영 변경을 계획·실행·검증하는 오픈소스 운영 플랫폼.

그래프는 기반 모델이다. Deployment→Pod→Node 조회 자체는 제품의 최종 가치가 아니다. 핵심은 여러 시스템에 나뉜 의존성과 관측 증거를 연결하고, 이를 진단과 운영 작업에 사용하는 것이다.

프로비저닝은 핵심 장기 과제이며 구현 순서상 뒤에 둔다. 외부 LLM 호출 지원은 API의 초기 설계 요구다. 모델 실행이나 특정 에이전트 프레임워크를 코어의 필수 의존성으로 두지 않는다. CLI, UI, 자동화 코드, 외부 AI agent가 같은 권한·검증 계약을 사용한다. MCP는 필요할 때 추가할 adapter이며 필수 프로토콜이 아니다.

현재 저장소의 임시 이름은 Opsplatform이며 경로는 github.com/Genzee/opsplatform이다. 아래 이름 후보는 이전 논의 기록이고 정식 브랜드 선택은 미정이다. 라이선스는 Apache-2.0으로 확정했다.

## 이름 후보

InfraGraph는 제품보다 내부 자료구조를 강조하므로 새 이름으로 대체한다. 사용자 선택 전 후보:

| 후보 | 발음 | 제안 의미 | 고려점 |
|---|---|---|---|
| Layerhelm | 레이어헬름 | 여러 인프라 계층을 조율하는 운영 플랫폼 | helm은 조타의 의미. Kubernetes Helm chart 관리 도구로 오해하지 않도록 설명 필요 |
| InfraHelm | 인프라헬름 | 인프라 운영을 조율하는 역할 | 더 직접적이나 Infra 접두어가 흔하고 Kubernetes Helm과 연상이 겹침 |

이번 공개 웹 검색에서 후보들과 같은 명칭의 뚜렷한 인프라 운영 플랫폼을 확인하지 못했다. 이는 이름·상표·도메인·패키지·GitHub organization의 가용성을 확정한 결과가 아니다. InfraHelm 문자열은 기존 GitHub 프로필 설명에도 나타난다. 검색 기록일: 2026-09-21.

검토 중 제외한 예: [InfraPilot](https://github.com/infrapilotinc/InfraPilot), [OpsWeave](https://github.com/slemens/opsweave), [Opsail](https://github.com/lencx/opsail)은 이미 관련 소프트웨어 프로젝트에서 쓰인다. 이름 확정 전 Go module이나 public API namespace를 새 이름으로 고정하지 않는다.

## 런타임

```text
CLI / UI / automation / external LLM agent
                    |
            Tool / CLI adapters
         (remote transport optional)
                    |
             Central Go server
      authentication / resource query
      reconciliation / identity / diagnosis
      plan and durable job control (later)
                    |
               PostgreSQL

Kubernetes provider --- observations ---> server
Linux node agents ----- observations ---> server
Storage/network providers -------------> server

server --- authorized durable jobs ---> executor workers (later)
```

초기는 중앙 서버 한 프로세스, PostgreSQL, 별도 provider process들이다. resource/evidence 저장과 dirty-work 등록은 원자적으로 처리한다. worker는 실패·재시작 후 DB의 작업 상태를 복구한다. discovery와 mutation executor의 credential은 분리한다. 별도 broker나 LLM orchestration runtime을 먼저 만들지 않는다.

## API는 운영 의미를 표현한다

공통 운영 service의 입출력 계약을 먼저 정의하고 CLI와 LLM tool adapter가 이를 호출한다. function tool에는 이름, 설명, 입력 schema와 실행 handler를 제공한다. OpenAPI는 필요하지 않으며 REST/gRPC는 원격 접근이 필요할 때 선택할 transport다. 현재 M1은 서비스 함수를 직접 호출하는 CLI와 JSON-lines tool adapter를 구현한다.

각 client는 필요한 operation subset을 해당 모델 SDK의 tool definition으로 감싸고 tool call을 handler에 전달한다. schema subset 차이와 향후 인증 전달은 client adapter에서 검증한다. 모든 기능을 매 요청마다 모델 context에 넣지는 않는다.

| Operation 예시 | 역할 | 시점 |
|---|---|---|
| `listResources`, `getResource` | native 리소스 검색·식별 | 초기 |
| `getDependencies`, `getImpact` | 의존성과 잠재 영향, 근거·관측 범위 | 초기 |
| `getEvidence` | 증거의 시각·출처·관측 한계 | 초기 |
| `createDiagnosis`, `getDiagnosis` | 증거를 모아 진단 job 실행·조회 | 첫 진단 경로 |
| `getCapabilities` | 배포된 provider/executor가 실제 지원하는 기능과 입력 schema | 단계적 |
| `createPlan`, `getPlan` | 지원하는 변경/프로비저닝 작업의 구체 계획 | 추후 |
| `executePlan`, `getOperation` | 권한·승인·현재 상태 검증 후 실행 및 추적 | 추후 |

실제 naming/path/schema는 API 설계 단계에서 고정한다. 한 개의 범용 `execute(command)` endpoint로 위 기능을 대체하지 않는다. getCapabilities는 scoped principal이 사용할 수 있는 기능을 알려주며, 조회 결과가 실행 시 authorization 검사를 대체하지 않는다.

진단 대상은 안정적인 resource ID와 시간 범위로 지정한다. 지원하지 않는 device/provider를 LLM이 요청하면 unsupported capability로 응답하고, 존재하지 않는 관측 결과를 생성하지 않는다.

## Agent가 사용하는 예

사용자: “payment-api가 느린데 스토리지 쪽 문제인지 확인해줘.”

1. agent가 workload를 검색한다. 같은 이름이 여러 cluster에 있으면 scope를 구분한다.
2. dependencies를 조회하여 Pod→volume→관측 가능한 스토리지 경로를 확인한다.
3. 필요한 시간 범위의 진단을 요청하고 job 상태를 조회한다.
4. 플랫폼은 원인 후보, 지지/반대 증거, 영향을 받는 리소스, 관측하지 못한 구간을 반환한다.
5. agent는 그 결과를 설명하고 필요한 추가 관측을 요청한다.
6. 추후 변경 기능이 있으면 concrete plan을 생성하고 권한·승인 정책을 거쳐 실행을 요청한다. discovery가 실행 결과를 재확인한다.

agent가 질문을 해석하고 여러 API 호출을 조합한다. 플랫폼은 식별, 그래프 traversal, 권한 검사, 작업 실행, 증거 기반 결과를 책임진다. 자연어 설명은 구조화된 증거와 결론 구분을 대체하지 않는다.

## 초기부터 지킬 API 계약

- 안정적 ID, versioned schema, 엄격한 입력 검증. name은 검색이고 execution target은 ID다.
- bounded response, pagination, summary + evidence reference. 대량 graph/로그를 무조건 반환하지 않는다.
- freshness, as_of, visibility boundary, missing evidence, truncation을 응답에 포함한다.
- finding은 observation / inference / hypothesis를 구분하고 evidence IDs를 연결한다. topology 연결만으로 causality를 확정하지 않는다.
- 긴 진단/실행은 job ID를 반환하고 polling으로 결과를 조회한다. 초기 SSE나 streaming은 필수로 하지 않는다.
- retryable job 생성·변경은 principal/operation 범위의 idempotency key와 request digest로 중복을 제어한다. 대상 외부 시스템에도 idempotency 또는 read-after-uncertain-result 복구가 필요하다. 외부 effect의 exactly-once를 무조건 약속하지 않는다.
- `AMBIGUOUS_RESOURCE`, `INSUFFICIENT_EVIDENCE`, `UNSUPPORTED_CAPABILITY`, `STALE_PLAN`, `APPROVAL_REQUIRED`처럼 client가 분기할 수 있는 error code와 retryable 정보를 둔다.
- plan에는 target IDs, desired change, preconditions, 영향, plan version/digest, expiry를 넣는다. 실행 전 현재 상태를 재검증하고 다르면 재계획을 요구한다.
- approval은 server가 검증하는 principal/policy 기록이며 client가 `approved: true`를 보내 획득하는 권한이 아니다. 승인 범위는 정확한 plan version에 묶는다. 별도 권한이 없는 agent는 자기 계획을 승인할 수 없다.
- audit에는 actor, delegated user, request ID, plan/job ID, 결과를 기록한다. 모델의 숨겨진 사고 과정 저장은 필요하지 않다.
- 수집한 log/annotation/description은 외부 데이터다. 그 안의 지시문으로 도구 권한·실행 정책을 바꿀 수 없다. 반환 자유 텍스트와 신뢰된 operation schema를 구분한다.

## 마일스톤 방향 보정

초기 문서의 identity, assertion, coverage, ordered ingestion, repository 계약은 기반으로 유지한다. 다만 “Kubernetes→Linux 그래프 출력”만으로 제품 데모를 완료했다고 보지 않는다.

첫 제품 경로 후보는 공유 스토리지 의존성과 장애 진단이다. Kubernetes/Linux에 필요한 스토리지 관측을 추가하여, 특정 노드의 연결 문제와 여러 노드가 공유하는 서버 장애를 근거로 구별한다. 이 구체 경로는 아직 범위 확정 전 제안이다.

물리 네트워크·스토리지 내부 원인까지 찾으려면 해당 provider의 증거가 필요하다. 원래 v0.1의 OS boundary만으로 모든 영역의 장애 원인을 특정할 수 있다고 약속하지 않는다. 초기에는 경계를 노출하고 지원 경로를 단계적으로 넓힌다.

사용자의 구현 시작 요청에 따라 공통 service와 CLI/tool adapter 기반 M1을 시작했다. 초기 범위는 fixture 기반 resource/relationship/provenance 및 조회이며 진단·provider protocol·실행은 아직 구현하지 않는다. 프로비저닝은 장기 목표로 유지하고 discovery provider와 executor를 분리한다. 현재 구현 범위와 제약은 루트 README를 기준으로 확인한다.
