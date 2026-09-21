# InfraGraph contracts v0.1 — design draft

Status: Proposed. 아래 코드는 인터페이스/스키마 초안이다. 컴파일·protoc·migration으로 검증한 구현물이 아니다. [설계](design-v0.1.md)의 semantics가 우선하며 구현 승인 후 executable contract로 전환한다.

## 1. 공통 데이터 규칙

- kind/type/evidence namespace는 open string. 등록된 namespace와 길이 제한을 검증하되 core enum에 provider 종류를 고정하지 않는다.
- resource key는 `(provider_id, kind, native_id)`. provider/scope는 인증 결과로 결정하며 client가 다른 scope를 주장할 수 없다.
- 한 resource는 하나의 scope에 소속한다. scope selector 변경은 새 baseline snapshot이 필요한 명시적 재구성이다.
- native revision은 opaque string. epoch/seq는 core의 ingestion ordering이고 native revision과 다르다.
- 시간은 UTC. observed_at은 source 제공값, received_at/first_seen_at/last_seen_at은 server clock. ordering에 observed_at을 쓰지 않는다. resource last_seen_at은 실제 관측 시각이며 scope coverage 확인 시각과 별개다.
- attribute는 JSON object이며 provider allowlist, 크기/깊이 제한, Secret 값 배제를 적용한다. 빈 map은 이전 attrs를 모두 제거한다.
- evidence는 typed namespace + normalized value + domain + source field. null/empty/placeholder는 matching evidence가 될 수 없다. raw original은 필요할 때만 제한된 접근으로 보관한다.
- source field/revision을 provenance에 기록하되 raw API response를 복사하지 않는다.
- 관계는 assertion이다. source/target refs가 아직 없을 수 있다. 조회는 resolved valid endpoints만 traversable로 취급한다.
- assertion state는 VALID/PENDING/STALE/RETRACTED. resource lifecycle과 별개이며 endpoint와 evidence의 현재 상태도 추가 검사한다.
- confidence는 optional descriptive number이고 자동 결합 임계값은 사용하지 않는다. resolution method와 evidence가 판정 근거다.

## 2. Go semantic interfaces

DTO는 `pkg/model`, event/client contract는 `pkg/providerapi`에 둔다. 아래는 공개 경계의 핵심 타입이며 validation/error/helpers는 생략했다.

```go
type Ref struct {
    ProviderID string
    Kind       string
    NativeID   string
}

type Evidence struct {
    Namespace      string // e.g. infragraph.identity.machine-id
    Domain         string // authenticated matching boundary
    Value          string // normalized value
    SourceField    string
    SourceRevision string
}

type ResourceObservation struct {
    Ref            Ref
    Name           string
    Attributes     map[string]any
    Evidence       []Evidence // replace the full evidence set for this resource
    ObservedAt     time.Time
    SourceRevision string
}

type Provenance struct {
    Method         string // observed, deterministic, explicit_binding
    SourceRef      Ref
    SourceField    string
    SourceRevision string
    RuleID         string
    RuleVersion    string
    Evidence       []EvidenceRef
}

type EvidenceRef struct {
    ResourceID string
    Namespace  string
    Digest     string
}

type Assertion struct {
    Key        string // stable, includes source/type/target within owner set
    Source     Ref
    Target     Ref
    Type       string
    Attributes map[string]any
    Provenance Provenance
}

type Projection struct {
    Owner       Ref
    Resources   []ResourceObservation // native object + derived children
    Assertions  []Assertion           // complete set owned by this owner
}

// For example a Pod projection owns the Pod, its runtime Container resources,
// Pod RUNS_ON Node and current Container relationships. It does not own Node.
// Replacing a projection tombstones previously owned children absent from it
// and retracts absent owned assertions atomically. Overlapping owners forbidden.
type Mutation struct {
    Replace *Projection
    Delete  *Ref // exactly one operation; deletes owner and its owned projection
}

type Session struct {
    ScopeID string
    Epoch   int64
    // Client knows the next sequence only through the acknowledged cursor.
}

type Batch struct {
    Session Session
    Seq     int64
    Changes []Mutation
}

type Ack struct {
    Epoch       int64
    AcceptedSeq int64
    Disposition string // applied, duplicate, resnapshot_required
}

type ProviderClient interface {
    Register(context.Context, Registration) (RegistrationResult, error)
    OpenScope(context.Context, ScopeSpec) (Session, error)
    Heartbeat(context.Context, HeartbeatRequest) error
    BeginSnapshot(context.Context, SnapshotBegin) (Ack, error)
    AppendSnapshot(context.Context, SnapshotChunk) (Ack, error)
    CompleteSnapshot(context.Context, SnapshotEnd) (Ack, error)
    AbortSnapshot(context.Context, SnapshotAbort) (Ack, error)
    Apply(context.Context, Batch) (Ack, error)
}

type Provider interface {
    Run(context.Context, ProviderClient) error
}

type Collector interface {
    Collect(context.Context) (Collection, error)
}
type Collection struct {
    Projections []Projection
    Complete    bool
    Diagnostics []Diagnostic // forbidden, race, unavailable, unsupported
}

// internal contracts: no provider imports, no raw SQL exposed to providers.
type Repository interface {
    Update(context.Context, func(WriteTx) error) error
    View(context.Context, func(ReadTx) error) error
}
type ReadTx interface {
    Get(context.Context, string) (Resource, error)
    Resolve(context.Context, Ref) (Resource, error)
    List(context.Context, Filter, Page) (ResourcePage, error)
    Adjacent(context.Context, []string, EdgeFilter, Page) (EdgePage, error)
    LookupEvidence(context.Context, EvidenceLookup) ([]EvidenceRecord, error)
    Coverage(context.Context, []string) ([]ScopeCoverage, error)
}
type WriteTx interface {
    ReadTx
    FenceScope(context.Context, ScopeGrant) (Session, error)
    ApplyOrdered(context.Context, Batch) (Ack, error)
    StageSnapshot(context.Context, SnapshotChunk) (Ack, error)
    PublishSnapshot(context.Context, SnapshotEnd) (Ack, error)
    EnqueueResolution(context.Context, []EvidenceLookup) error
    ReplaceDerivedAssertions(context.Context, ResolutionResult) error
}

type Resolver interface {
    Resolve(context.Context, ReadTx, ResolutionRule, EvidenceLookup) (ResolutionResult, error)
}
type QueryService interface {
    GetResource(context.Context, string) (Resource, error)
    ListResources(context.Context, Filter, Page) (ResourcePage, error)
    Traverse(context.Context, TraversalRequest) (GraphResult, error)
    FindPath(context.Context, PathRequest) (GraphResult, error)
}
```

Repository Update transaction 안에서 외부 네트워크를 호출하지 않는다. query는 하나의 ReadTx에서 완료한다. memory는 copy-on-write read view, PostgreSQL은 read-only repeatable-read transaction을 후보로 두고 장시간 snapshot의 비용을 측정한다.

`Projection`은 Pod 갱신에 따른 Container와 edge 삭제를 원자적으로 처리하기 위한 단위다. 범용 attribute patch, 임의 SQL, 일반 relationship ownership migration은 제공하지 않는다.

## 3. Provider wire protocol draft

Transport는 gRPC이며 semantic operation은 위 Go 계약과 같다. 아래는 주요 protobuf message/RPC 초안이다. Resource ID는 core가 생성하며 provider는 Ref를 보낸다.

```proto
syntax = "proto3";
package infragraph.provider.v1alpha1;

import "google/protobuf/struct.proto";
import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";

service ProviderAPI {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc OpenScope(OpenScopeRequest) returns (Session);
  rpc Heartbeat(HeartbeatRequest) returns (google.protobuf.Empty);
  rpc Publish(PublishRequest) returns (Ack);
}

message RegisterRequest {
  string protocol_version = 1;
  string provider_type = 2;
  string software_version = 3;
  string enrollment_key = 4; // identity hint; never authorization
  repeated string supported_kinds = 5;
}
message RegisterResponse {
  string provider_id = 1;
  string correlation_domain = 2; // server assigned
  uint32 max_batch_bytes = 3;
  uint32 max_mutations = 4;
}
message OpenScopeRequest {
  string provider_id = 1;
  string collection_key = 2;
  string selector_fingerprint = 3;
  repeated string kinds = 4;
}
message Session {
  string scope_id = 1;
  int64 epoch = 2;
  int64 next_seq = 3;
}
message ScopeHealth {
  string scope_id = 1;
  int64 epoch = 2;
  string state = 3; // healthy, partial, failed, resyncing
  string reason_code = 4;
  int64 covered_through_seq = 5;
  bool continuity_confirmed = 6;
}
message HeartbeatRequest {
  string provider_id = 1;
  repeated ScopeHealth scopes = 2;
}
message Ref {
  string provider_id = 1;
  string kind = 2;
  string native_id = 3;
}
message Evidence {
  string namespace = 1;
  string domain = 2;
  string value = 3;
  string source_field = 4;
  string source_revision = 5;
}
message Resource {
  Ref ref = 1;
  string name = 2;
  google.protobuf.Struct attributes = 3;
  repeated Evidence identity_evidence = 4;
  google.protobuf.Timestamp observed_at = 5;
  string source_revision = 6;
}
message EvidenceRef {
  string resource_id = 1;
  string namespace = 2;
  string digest = 3;
}
message Provenance {
  string method = 1;
  Ref source_ref = 2;
  string source_field = 3;
  string source_revision = 4;
  string rule_id = 5;
  string rule_version = 6;
  repeated EvidenceRef input_evidence = 7;
}
message Assertion {
  string key = 1;
  Ref source = 2;
  Ref target = 3;
  string type = 4;
  Provenance provenance = 5;
  google.protobuf.Struct attributes = 6;
}
message Projection {
  Ref owner = 1;
  repeated Resource resources = 2;
  repeated Assertion assertions = 3;
}
message Mutation {
  oneof operation {
    Projection replace_projection = 1;
    Ref delete_projection = 2;
  }
}
message Delta { repeated Mutation mutations = 1; }
message BeginSnapshot { string snapshot_id = 1; }
message SnapshotChunk {
  string snapshot_id = 1;
  uint32 chunk_index = 2;
  repeated Projection projections = 3;
}
message CompleteSnapshot {
  string snapshot_id = 1;
  uint32 chunk_count = 2;
  uint64 projection_count = 3;
}
message AbortSnapshot {
  string snapshot_id = 1;
  string reason_code = 2;
}
message PublishRequest {
  string scope_id = 1;
  int64 epoch = 2;
  int64 seq = 3;
  oneof payload {
    Delta delta = 4;
    BeginSnapshot begin = 5;
    SnapshotChunk chunk = 6;
    CompleteSnapshot complete = 7;
    AbortSnapshot abort = 8;
  }
}
message Ack {
  int64 epoch = 1;
  int64 accepted_seq = 2;
  string disposition = 3;
}
```

인증 credential은 metadata/TLS로 전달한다. payload의 provider_id는 인증 결과와 일치해야 한다. 등록은 익명 자동 enrollment가 아니다. server policy가 kind namespace, collection scope, resource/byte budget을 부여한다. builtin resolver만 reserved identity로 derived assertion을 발행할 수 있다.

protobuf Struct에 큰 정수의 정확성을 의존하지 않는다. identity, PID start ticks, device identifier, resourceVersion은 string으로 보존한다. transport state에는 enum을 쓸 수 있지만 resource kind/type은 string이다. v1alpha1의 breaking change는 명시하고 stable version의 삭제된 field number는 reserved로 지정한다.

### Ordering / retry / snapshot contract

1. OpenScope는 writer lease policy에 따라 epoch를 발급한다. active writer가 있으면 다른 writer를 거부하며 lease 만료 또는 명시적 replacement 때만 fence한다. 새 session은 seq 1부터 시작하고 snapshot이 필수다.
2. Publish는 scope row를 lock하고 epoch/seq를 검증하여 전체 batch를 한 transaction에서 수락한다. invalid/unsupported mutation이 있으면 부분 적용하지 않는다.
3. 동일 epoch/seq와 semantic payload hash는 duplicate ACK. 같은 seq에 다른 payload는 INVALID_ARGUMENT. gap은 FAILED_PRECONDITION과 expected_seq를 반환하며 만료 epoch도 거부한다.
4. retry digest는 unknown field를 제외한 versioned canonical semantic serialization으로 계산한다. map key 순서, time 표현, numeric string 표현을 고정한 golden vector를 추가한다. 단순 protobuf bytes 비교에 의존하지 않는다.
5. replay receipt는 유한하게 보관한다. cursor보다 오래되었고 receipt가 GC된 seq는 재적용하지 않고 resnapshot_required를 반환한다. provider는 새 epoch로 복귀한다.
6. snapshot 중에는 live state 대신 staging에 기록한다. complete 시 chunk index/count, owner uniqueness, scope ownership을 검사한다. complete의 publish와 absence tombstone은 하나의 transaction이다.
7. abort/timeout/disconnect 시 이전 live state를 유지한다. 미완료 snapshot은 coverage를 healthy로 바꾸지 않는다. snapshot bytes/projections/duration에 상한을 둔다.
8. delta는 baseline complete 이후만 허용한다. projection 교체 시 빠진 owned children/assertions를 철회한다. source read가 partial이면 해당 projection 전체를 보내지 않고 coverage failure로 보고한다.
9. seq ACK와 data write는 같은 transaction이다. DB timeout이나 commit 결과 불명 시 동일 request를 retry한다. RESOURCE_EXHAUSTED는 backoff하며 인증 실패를 무한 재시도하지 않는다.
10. heartbeat의 coverage 갱신은 current epoch, healthy continuity, `covered_through_seq == accepted_seq`, 유효 baseline 조건에서만 허용한다. 새로운 resource observation 시각을 만들어내지 않는다.

## 4. REST / CLI contract draft

REST query는 read-only, provider write는 gRPC만 사용한다. candidate 승인/manual binding write API는 초기 필수가 아니다. explicit binding은 관리 설정으로 제공할 수 있다.

| HTTP | Semantics |
|---|---|
| GET `/v1alpha1/resources/{id}` | resource、native health、freshness、scope、provenance references |
| GET `/v1alpha1/resources` | provider/kind/namespace/name/state 필터, cursor pagination |
| GET `/v1alpha1/resources/{id}/neighbors` | direction=in/out/both、edge type filter |
| GET `/v1alpha1/resources/{id}/dependencies` | dependency profile 정방향 |
| GET `/v1alpha1/resources/{id}/dependents` | dependency profile 역방향 |
| GET `/v1alpha1/resources/{id}/ancestors` | ownership profile 역방향 |
| GET `/v1alpha1/resources/{id}/descendants` | ownership profile 정방향 |
| GET `/v1alpha1/resources/{id}/infrastructure` | infrastructure/v1 profile |
| GET `/v1alpha1/resources/{id}/impact` | potential-impact/v1, 설명을 포함한 잠재 영향 |
| GET `/v1alpha1/path?source={id}&target={id}` | bounded BFS、direction指定、stable tie-break |
| GET `/v1alpha1/relationships/{assertion_id}` | exact assertion、provenance、resolution reason |
| GET `/v1alpha1/providers` | connections、scope coverage、last errors |
| GET `/v1alpha1/resolution-candidates` | rule・候補・拒否/曖昧理由 |

공통 query params의 기본값 후보는 `include_stale=false`, `max_depth=16`, `max_nodes=1000`, `max_edges=5000`이다. server-side ceiling과 2초 query budget을 두고 실측 후 조정한다. 무제한 traversal은 제공하지 않는다. filter/ID는 parameterized query로 처리하며 client SQL은 받지 않는다.

```json
{
  "root": "resource-uuid",
  "profile": "potential-impact/v1",
  "as_of": "2026-09-21T12:00:00Z",
  "semantics": "potential-impact-not-outage",
  "resources": [],
  "relationships": [],
  "paths": [],
  "truncated": false,
  "incomplete_reasons": ["linux-mount-collector-forbidden"],
  "frontiers": [{"resource_id": "resource-uuid", "reason": "stale-edge"}]
}
```

404는 미지 ID, 410은 retained tombstone을 일반 resource로 요청한 경우, 400은 invalid filter/profile, 401/403은 인증/권한 실패, 429는 quota, 503은 repository 장애다. 탐색 budget에 도달한 부분 결과는 200+truncated이며 DB 장애를 정상적인 빈 graph로 변환하지 않는다.

pagination은 opaque cursor와 안정적인 `(kind,name,id)` 순서를 사용한다. 각 page는 서로 다른 관측 시점일 수 있음을 표시하며 전체 page의 일괄 snapshot을 보장하지 않는다. 단일 traversal은 하나의 read snapshot에서 완료한다.

CLI sugar:

```text
infra resources --kind kubernetes.Pod
infra get <uuid>
infra trace deployment production/payment-api --cluster <scope>
infra impact linux-os/worker-01 --cluster <scope>
infra neighbors <uuid> --direction both
infra path <source-uuid> <target-uuid>
infra providers
infra replay <fixture> --offline
```

kind alias/name은 CLI 검색으로 UUID에 대응시킨다. 여러 후보가 있으면 후보 목록과 실패 exit를 반환하고 첫 항목을 임의 선택하지 않는다. trace/impact의 `--explain`은 freshness/provenance를 펼친다. replay 기본 대상은 격리된 in-memory graph이며 live server는 명시 target이 있어야 한다.

## 5. PostgreSQL logical schema draft

아래는 주요 table/constraint/index DDL 초안이며 migration으로 실행하지 않았다. UUID는 application이 생성하고 timestamptz는 UTC를 사용한다. lifecycle 등 core state는 text CHECK, kind/type은 열린 text다.

```sql
CREATE TABLE providers (
  id uuid PRIMARY KEY,
  provider_type text NOT NULL,
  enrollment_key text NOT NULL UNIQUE,
  correlation_domain text NOT NULL,
  software_version text NOT NULL,
  last_heartbeat_at timestamptz,
  registered_at timestamptz NOT NULL
);

CREATE TABLE collection_scopes (
  id uuid PRIMARY KEY,
  provider_id uuid NOT NULL REFERENCES providers(id),
  collection_key text NOT NULL,
  selector_fingerprint text NOT NULL,
  descriptor jsonb NOT NULL,
  epoch bigint NOT NULL DEFAULT 0 CHECK (epoch >= 0),
  accepted_seq bigint NOT NULL DEFAULT 0 CHECK (accepted_seq >= 0),
  writer_lease_until timestamptz,
  baseline_valid boolean NOT NULL DEFAULT false,
  coverage_state text NOT NULL,
  coverage_reason text,
  coverage_confirmed_at timestamptz,
  last_snapshot_at timestamptz,
  UNIQUE (provider_id, collection_key),
  UNIQUE (id, provider_id)
);

CREATE TABLE resources (
  id uuid PRIMARY KEY,
  provider_id uuid NOT NULL REFERENCES providers(id),
  scope_id uuid NOT NULL,
  kind text NOT NULL,
  native_id text NOT NULL,
  owner_kind text NOT NULL,
  owner_native_id text NOT NULL,
  name text NOT NULL,
  attributes jsonb NOT NULL CHECK (jsonb_typeof(attributes) = 'object'),
  source_revision text,
  observed_at timestamptz,
  first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  lifecycle text NOT NULL CHECK (lifecycle IN ('ACTIVE','STALE','DELETED')),
  deleted_at timestamptz,
  last_epoch bigint NOT NULL,
  last_seq bigint NOT NULL,
  UNIQUE (provider_id, kind, native_id),
  FOREIGN KEY (scope_id, provider_id) REFERENCES collection_scopes(id, provider_id)
);
CREATE INDEX resources_kind_idx ON resources(kind);
CREATE INDEX resources_name_idx ON resources(provider_id,kind,name,id);
CREATE INDEX resources_owner_idx ON resources(scope_id,owner_kind,owner_native_id);
CREATE INDEX resources_state_idx ON resources(scope_id,lifecycle);

CREATE TABLE identity_evidence (
  id uuid PRIMARY KEY,
  resource_id uuid NOT NULL REFERENCES resources(id),
  namespace text NOT NULL,
  domain text NOT NULL,
  value text NOT NULL,
  digest text NOT NULL,
  source_field text NOT NULL,
  source_revision text,
  observed_at timestamptz,
  last_seen_at timestamptz NOT NULL,
  UNIQUE (resource_id,namespace,domain,value,source_field)
);
CREATE INDEX evidence_lookup_idx ON identity_evidence(domain,namespace,value);

CREATE TABLE relationships (
  id uuid PRIMARY KEY,
  provider_id uuid NOT NULL REFERENCES providers(id),
  scope_id uuid NOT NULL,
  owner_kind text NOT NULL,
  owner_native_id text NOT NULL,
  assertion_key text NOT NULL,
  source_ref jsonb NOT NULL,
  target_ref jsonb NOT NULL,
  source_resource_id uuid REFERENCES resources(id),
  target_resource_id uuid REFERENCES resources(id),
  type text NOT NULL,
  attributes jsonb NOT NULL,
  provenance jsonb NOT NULL,
  confidence double precision CHECK (confidence >= 0 AND confidence <= 1),
  state text NOT NULL CHECK (state IN ('VALID','PENDING','STALE','RETRACTED')),
  first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  retracted_at timestamptz,
  last_epoch bigint NOT NULL,
  last_seq bigint NOT NULL,
  UNIQUE (scope_id,owner_kind,owner_native_id,assertion_key),
  FOREIGN KEY (scope_id, provider_id) REFERENCES collection_scopes(id, provider_id)
);
CREATE INDEX relationships_source_idx ON relationships(source_resource_id,type,state);
CREATE INDEX relationships_target_idx ON relationships(target_resource_id,type,state);
CREATE INDEX relationships_type_idx ON relationships(type);
CREATE INDEX relationships_owner_idx ON relationships(scope_id,owner_kind,owner_native_id);
CREATE INDEX pending_source_idx ON relationships
  ((source_ref->>'provider_id'),(source_ref->>'kind'),(source_ref->>'native_id'))
  WHERE state = 'PENDING';
CREATE INDEX pending_target_idx ON relationships
  ((target_ref->>'provider_id'),(target_ref->>'kind'),(target_ref->>'native_id'))
  WHERE state = 'PENDING';

CREATE TABLE relationship_inputs (
  relationship_id uuid NOT NULL REFERENCES relationships(id),
  resource_id uuid NOT NULL REFERENCES resources(id),
  evidence_namespace text NOT NULL,
  evidence_digest text NOT NULL,
  PRIMARY KEY (relationship_id,resource_id,evidence_namespace,evidence_digest)
);
CREATE INDEX relationship_inputs_resource_idx ON relationship_inputs(resource_id);

CREATE TABLE resolution_candidates (
  id uuid PRIMARY KEY,
  rule_id text NOT NULL,
  rule_version text NOT NULL,
  source_resource_id uuid NOT NULL REFERENCES resources(id),
  target_resource_id uuid NOT NULL REFERENCES resources(id),
  state text NOT NULL,
  reason_code text NOT NULL,
  evidence jsonb NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (rule_id,source_resource_id,target_resource_id)
);

CREATE TABLE ingest_receipts (
  scope_id uuid NOT NULL REFERENCES collection_scopes(id),
  epoch bigint NOT NULL,
  seq bigint NOT NULL,
  payload_hash text NOT NULL,
  received_at timestamptz NOT NULL,
  PRIMARY KEY (scope_id,epoch,seq)
);

CREATE TABLE snapshots (
  id uuid PRIMARY KEY,
  scope_id uuid NOT NULL REFERENCES collection_scopes(id),
  epoch bigint NOT NULL,
  state text NOT NULL CHECK (state IN ('OPEN','COMPLETE','ABORTED')),
  created_at timestamptz NOT NULL,
  completed_at timestamptz
);
CREATE UNIQUE INDEX one_open_snapshot_idx ON snapshots(scope_id) WHERE state='OPEN';
CREATE TABLE snapshot_chunks (
  snapshot_id uuid NOT NULL REFERENCES snapshots(id),
  chunk_index integer NOT NULL CHECK (chunk_index >= 0),
  projections jsonb NOT NULL,
  PRIMARY KEY (snapshot_id,chunk_index)
);

CREATE TABLE resolution_work (
  domain text NOT NULL,
  namespace text NOT NULL,
  value text NOT NULL,
  generation bigint NOT NULL,
  queued_at timestamptz NOT NULL,
  attempts integer NOT NULL DEFAULT 0,
  PRIMARY KEY (domain,namespace,value)
);
```

### DB semantics

- resource unique key에 name은 포함하지 않는다. kind와 native key는 immutable이다. 일반 upsert로 scope를 변경할 수 없다.
- 미지 endpoint에 fake resource row를 만들지 않는다. refs를 먼저 저장하고 pending index로 후착 resource를 연결한다. resolved ID와 Ref의 일치는 ingestion에서 검증한다.
- assertion key는 endpoints/type을 포함한다. endpoint 교체는 기존 assertion 철회와 새 assertion 생성이다. logical relationship은 read projection에서 집계한다.
- provenance는 JSON이지만 derived edge의 입력 의존성은 relationship_inputs에 기록한다. evidence 변경 시 invalidation과 dirty enqueue를 같은 transaction에서 수행한다.
- dirty-key는 generation을 검사하여 compare-and-delete한다. 처리 중 새 갱신이 들어온 queue entry를 삭제하지 않는다. core restart 후 남은 작업을 처리한다.
- resource가 fresh여도 scope가 stale이면 effective lifecycle은 STALE이다. materialized lifecycle 갱신 지연만으로 fresh로 판단하지 않는다. derived assertion은 input resource/scope/digest도 검사한다.
- current-state store이며 완전한 역사 DB가 아니다. tombstone/receipt retention은 유한하고 현재 사용 중인 resource를 나이만으로 삭제하지 않는다.
- retention GC는 retracted assertion의 inputs를 먼저 정리하고 assertion/candidate, 참조 없는 tombstone의 evidence/resource 순서로 transaction 안에서 처리한다. 현재 assertion이 참조하는 tombstone은 유지하거나 assertion을 먼저 철회한다. cascade로 provenance를 무언중 삭제하지 않는다.
- snapshot staging/receipts는 재처리 경계이며 무기한 event log가 아니다. 이전 epoch는 receipt GC 후에도 scope epoch로 거부한다. 같은 epoch의 오래된 request는 cursor로 막고 resnapshot을 요구한다.
- native incarnation tombstone GC 후에도 같은 epoch의 오래된 upsert는 seq로 차단한다. source 자체의 잘못된 재발견은 native UID/generation과 provider 계약으로 막는다. core가 source의 진실을 독립적으로 증명할 수는 없다.
- scope row → resource → relationship → identity work 순으로 lock ordering을 정한다. resolver는 충돌 시 retry하고 input version을 재확인한다.
- 처음부터 attributes 전체에 GIN index를 만들지 않는다. 실제 name/namespace/evidence query에 필요한 expression/B-tree index를 추가한다. metadata filter에도 pagination/budget을 적용한다.
- `SELECT *`로 전체 graph를 메모리에 가져오는 query는 제공하지 않는다. frontier별 adjacency lookup을 batch로 수행하며 index와 visited 상한을 검증한다.

## 6. Rule/profile extension contract

Provider 등록으로 core behavior를 임의 변경할 수 없다. kind namespace 등록은 가능하지만 resolution rule/query profile은 operator-approved configuration이다.

```yaml
id: kubernetes-node-linux-os
version: "1"
source_kind: kubernetes.Node
target_kind: linux.OS
correlation_domain: authenticated
evidence:
  - infragraph.identity.machine-id
  - infragraph.identity.system-uuid
match: exact_unique
require_one_shared_valid_identifier: true
reject_conflicting_shared_identifiers: true
require_unique_each_side_by_kind: true
relationship: BACKED_BY
```

위 내용은 형식 초안이며 범용 rule language 구현 지시가 아니다. machine-id/System UUID 각각의 후보 집합을 계산하고 공유하는 유효 identifier가 모두 같은 target을 가리키며 양쪽의 같은 kind 후보가 유일할 때만 연결한다. 중복 OS가 STALE이어도 충돌 증거로 유지되는 동안 자동 연결을 보류한다. 명시적 retirement/binding으로 해소할 수 있다.

known-conflict 리소스를 단순 freshness filter로 후보에서 숨겨 unique로 취급하지 않는다. 아직 발견하지 못한 duplicate까지 탐지한다고 주장하지 않는다. hostname 후보는 같은 domain으로 제한하고 후보 수에 상한을 둔다.
