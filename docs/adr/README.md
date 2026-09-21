# ADR drafts 001–010

모두 **Proposed**, 2026-09-21. 검토 편의를 위해 한 파일에 묶었다. 승인 시 개별 ADR 파일로 분리하고 결정일·검토자를 기록한다. [설계 리뷰](../architecture/design-v0.1.md), [계약](../architecture/contracts-v0.1.md)을 함께 읽는다.

## ADR-001 — Resource identity model

Context: names, PID, runtime slots는 재사용된다. provider process 재시작은 리소스 identity를 바꾸면 안 된다.

Decision proposed: core 발급 UUID + unique `(logical provider, concrete kind, native incarnation ID)`. Kubernetes UID를 사용하고 Cluster는 kube-system UID 기반 scope를 기본으로 한다. LinuxOS는 enrollment + OS generation, ephemeral OS child는 boot/namespace/native lifetime/generation을 포함한다. Node와 OS는 merge하지 않는다.

Alternatives: human-readable global path는 재생성에 취약. native ID만 external ID로 쓰면 provider key 형식이 API에 노출. canonical universal Host는 서로 다른 lifecycle을 합친다.

Consequences: 조회용 alias/name resolution이 필요하다. enrollment state 유실 시 중복 candidate가 생길 수 있으나 오합병보다 안전하다. fixture에 재생성/reboot/PID reuse를 포함한다.

Review gate: LinuxOS 재설치와 enrollment 재발급의 사용자 경험, cluster clone 충돌 처리.

## ADR-002 — Relationship and provenance model

Context: 같은 edge에 여러 관측 근거가 있고 각 근거는 독립적으로 철회될 수 있다. ownership와 runtime dependency는 다른 의미다.

Decision proposed: directed typed assertion을 writer scope별로 보관한다. assertion은 source/target refs, owner, method, source field/revision, rule version, evidence digest, timestamps를 갖는다. query에서 logical edge를 집계한다. resource/owned child/assertion set은 projection 단위로 교체한다. missing endpoint는 pending이다.

Alternatives: 단일 mutable edge는 provider 간 삭제 충돌. 모든 관계를 DEPENDS_ON으로 통일하면 native 의미 소실. 전체 관측 event history는 storage/retention 범위가 커진다.

Consequences: assertion과 logical edge의 구분을 API 문서에 명시해야 한다. tombstone/철회와 evidence invalidation을 구현해야 한다. confidence는 선택적 설명 값이며 신뢰의 대체물이 아니다.

Review gate: Pod USES PVC와 Container MOUNTS PVC의 선언/관측 차이, profile별 impact 규칙.

## ADR-003 — Provider protocol

Context: network retry, out-of-order, old writer, partial snapshot에 대한 명시적 계약이 필요하다.

Decision proposed: transport-independent semantic operations + gRPC adapter. authenticated provider registration, collection scopes, core-issued fenced epoch, contiguous sequence, transactional ACK, staged complete snapshots. initial/restart snapshot 후 ordered delta. bounded receipts; receipt 유실 구간은 resnapshot.

Alternatives: timestamp LWW는 skew에 취약. Kafka/event log는 초기 운영 비용이 크다. 완전한 per-object vector clock은 single-writer scope에서 불필요하다.

Consequences: scope당 한 writer, provider-side cache projection serialization 필요. partial snapshot은 publish하지 않는다. 큰 scope는 batch staging과 resource limits로 제한한다. transport를 바꿔도 동일 fixture trace를 통과해야 한다.

Review gate: canonical payload hashing, lease takeover 및 timeout default, 최대 snapshot 크기.

## ADR-004 — Resource lifecycle and stale semantics

Context: 관측되지 않음, 삭제됨, provider 끊김, native unhealthy는 다른 사실이다.

Decision proposed: ACTIVE/STALE/DELETED; native health는 attributes. heartbeat와 scope coverage를 분리. authoritative delete 또는 complete snapshot absence로만 DELETED. coverage 실패/grace 경과는 STALE, 자동 TTL delete 없음. 관계에도 pending/stale/retracted lifecycle을 둔다. stale input에서 derived edge를 fresh로 취급하지 않는다.

Alternatives: missing heartbeat 후 전체 delete는 장애 때 graph를 잃는다. 영구 ACTIVE는 오정보. 모든 unchanged resource refresh는 불필요한 쓰기 증가.

Consequences: query가 effective freshness를 계산하고 missing coverage를 설명해야 한다. current graph + bounded tombstone만 유지하며 historical time travel을 약속하지 않는다.

Review gate: stale/tombstone retention 값과 CLI default stale 표시 방식.

## ADR-005 — Kubernetes ↔ Linux identity resolution

Context: Node와 OS는 다른 객체이며 machine-id/system UUID도 복제·누락될 수 있다. runtime ID와 cgroup path는 별도 검증이 필요하다.

Decision proposed: Node→OS는 authenticated domain, valid normalized exact evidence, kind별 uniqueness, shared evidence contradiction 없음이라는 조건으로 BACKED_BY assertion 생성. hostname은 candidate만. Container→Cgroup은 Node→OS가 먼저 검증된 범위에서 full runtime ID exact match. native resources를 merge하지 않는다. rule version과 evidence digest를 보존하고 변화 시 철회한다.

Alternatives: hostname match는 동명이인. 단일 machine-id 무조건 match는 복제 문제. 매번 manual binding은 automatic topology 요구를 충족하지 못한다.

Consequences: 자동 graph에 gap이 생길 수 있고 이유를 노출해야 한다. numeric confidence score보다 rejection reason이 중요하다. cgroup layout adapter의 지원 범위를 문서화한다.

Review gate: machine-id 또는 UUID 하나만 제공되는 환경의 자동 매칭 허용, cloned identifier 격리 정책.

## ADR-006 — PostgreSQL graph persistence

Context: current-state resource graph와 transactional ingestion, identity indexes가 필요하다. query workload는 아직 검증되지 않았다.

Decision proposed: PostgreSQL source of truth, JSONB attributes/provenance, B-tree adjacency/evidence indexes. resources/assertions/scopes/evidence/snapshot staging/receipts/dirty work/candidates. repository interfaces 뒤에 memory와 PostgreSQL 구현. single core writer로 시작. data+ACK cursor+dirty enqueue를 같은 transaction에 기록.

Alternatives: graph DB는 설치·운영 비용 추가. embedded DB만으로 primary Kubernetes deployment와 durable central graph를 대체하기 어려움. event sourcing은 replay와 retention 복잡성 증가.

Consequences: bounded adjacency traversal과 query budget이 필요하다. change log/projection into graph DB는 미래 실측 필요 시 설계한다. multi-replica core HA는 v0.1 보장 범위 밖이며 이를 설치 문서에 밝힌다.

Review gate: SQL contract tests, indexes EXPLAIN, crash consistency, measured scale envelope.

## ADR-007 — Provider plugin/isolation strategy

Context: third-party provider 추가에 core rebuild가 필요하면 생태계 확장이 막힌다. 초기 dynamic plugin ABI는 과도하다.

Decision proposed: 처음부터 out-of-process provider binaries와 versioned protocol. core는 unknown kind/type을 저장한다. provider-specific discovery는 provider에만 둔다. generic resolver와 query는 operator-approved declarative rules/profiles를 읽는다. 새 provider가 특수한 derivation이 필요하면 별도 authorized resolver producer 가능.

Alternatives: Go plugin ABI는 binary coupling. core에 모든 provider import는 isolation 위반. 임의 코드 rule engine은 attack surface와 유지보수 부담.

Consequences: 새 kind 저장과 그 kind의 semantic trace 지원은 별개의 capability다. third-party provider가 impact semantics를 임의 변경할 수 없다. single Go module로 시작하되 공개 SDK는 internal에 의존하지 않는다.

Review gate: rule/profile validation, reserved namespaces, dependency boundary CI.

## ADR-008 — Linux privilege/security model

Context: proc/sys/netlink의 visibility는 namespace와 권한에 좌우된다. host access가 필요한 DaemonSet은 cluster security policy와 충돌할 수 있다.

Decision proposed: read-only discovery, no SSH/shell/runtime socket, capabilities drop ALL을 baseline 목표로 한다. hostNetwork, 제한된 read-only host paths, 좁은 enrollment state write path. hostPID는 기본 false로 시작해 지원 환경 테스트. permission denied는 partial coverage. TLS+provider scope auth, attrs allowlist, read API authentication 필수.

Alternatives: privileged=true 기본은 불필요한 권한. 순수 restricted Pod는 host discovery에 부족. CRI socket mount는 read-only filesystem flag로 API mutation을 막을 수 없다.

Consequences: distro/LSM/hidepid별 지원 행렬이 필요하다. 완전한 process/cgroup discovery가 baseline에서 불가능한 환경은 제한을 공개하고 추가 권한 profile을 별도로 검토한다. Linux privileged namespace 접근 없이 물리 host를 추정하지 않는다.

Review gate: 실제 VM에서 최소 권한과 coverage, token node binding, credential rotation, Secret 유출 fixture.

## ADR-009 — CNCF/OpenTelemetry semantic reuse

Context: 속성 naming과 service/host/container identity를 모두 새로 만들 필요가 없다. OTel Resource는 여러 entity 속성을 담을 수 있어 그래프 노드와 1:1이 아니다.

Decision proposed: `k8s.*`, `container.id`, `host.*`, `os.*` 등 의미가 일치하는 conventions를 재사용하고 adopted semconv version을 기록한다. host.id와 모든 machine/system identifier를 같은 값으로 뭉치지 않는다. 별도 typed identity evidence namespace와 source를 유지한다. Kubernetes UID/ownerReference/informer를 재사용한다. OTel/Prometheus는 InfraGraph 자체 telemetry에 사용한다.

Alternatives: convention 전체 종속은 변경 영향이 크다. 전부 custom key는 연동 비용. OTLP를 graph write protocol로 쓰면 snapshot/delete/coverage 계약이 불명확해진다.

Consequences: semconv mapping table과 version upgrade policy가 필요하다. 미확정 entity 표준은 참고하되 core protocol의 필수 runtime dependency로 만들지 않는다. high-volume telemetry를 graph DB에 저장하지 않는다.

Review gate: initial attribute allowlist, host.id 수집 정책, semconv pin version.

## ADR-010 — Future Desired Graph compatibility

Context: 향후 provisioning을 열어두되 v0.1 discovery 범위가 planner/executor로 확대되면 안 된다.

Decision proposed: actual graph는 observation만 소유한다. 미래 desired graph/intent/plan은 별도 domain과 상태 machine을 갖고 actual IDs/provenance를 참조한다. execution adapters는 Kubernetes/Argo/Crossplane 등을 사용할 수 있다. v0.1에 operation engine, desired flag, shell execution을 추가하지 않는다.

Alternatives: actual resource에 desired fields 추가는 책임 혼합. 지금 planner skeleton 작성은 검증되지 않은 추상화. 확장 불가능한 hard-coded kind enum도 배제한다.

Consequences: immutable identity, namespaced extensibility, read API가 미래 연결점이다. 지금 planner schema를 고정하지 않으며 multi-tenancy/history/HA도 별도 ADR 대상이다.

Review gate: 현재 필드에 desired/provisioning semantics가 새어들지 않았는지 확인.
