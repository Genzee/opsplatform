# Provider and core test contracts

Status: Proposed. 구현 후 갖춰야 할 검증 계약이다. 현재 실행한 테스트 결과가 아니다.

## 1. Fixture format

```text
providers/kubernetes/testdata/<scenario>/
  input/*.json                 native objects with synthetic UIDs
  events.yaml                  add/update/delete/cache-sync sequence
  expected-projections.json
  expected-queries.json

providers/linux/testdata/<scenario>/
  proc/
  sys/
  netlink.json                 recorded native message projection
  clock.json
  expected-projections.json
  expected-coverage.json

test/replay/<scenario>/
  providers.json
  protocol-events.jsonl
  expected-graph.json
  expected-query-results.json
```

Native collector와 native→projection 변환을 분리한다. fixture가 곧 live collector 검증을 대체하지는 않는다. Linux fixture reader는 injected FS/clock/netlink source를 사용한다. fixture 실행 중 host `/proc`나 실제 cluster에 fallback하면 실패한다.

ID는 canonical refs로 비교하고 core UUID는 test allocator를 주입한다. ordering이 의미 없는 배열은 canonical sort한다. timestamp는 fake clock을 사용한다. expected graph에는 resource뿐 아니라 assertion provenance, effective freshness, unresolved refs와 coverage reason을 포함한다.

수집 fixture에 실제 env/명령줄/token/mount password를 넣지 않는다. redaction 검증에는 합성 secret marker를 사용한다. 실제 수집 자료는 제출 전 anonymization과 provenance 검토를 거친다.

## 2. 공통 provider contract

모든 provider는 다음을 통과해야 한다.

| 입력/상황 | 기대 결과 |
|---|---|
| 동일 native snapshot 두 번 | 같은 native refs/relationship keys; 불필요한 새 identity 없음 |
| name 동일, native incarnation 다름 | 별도 resource |
| attributes/evidence 제거 | 이전 값 제거, derived edge invalidation |
| owner projection에서 child/edge 제거 | 그 owner가 소유한 child/edge만 tombstone/retract |
| 다른 owner/scope key 덮어쓰기 | validation 실패, batch 미적용 |
| endpoint 먼저 없는 relationship | pending, endpoint 도착 후 활성화 |
| endpoint authoritative delete | traversal에서 즉시 제외 |
| partial read / permission denied | complete snapshot 발행 금지, coverage failure |
| collect 중 객체 disappearance | native 특성에 맞는 retry/부분 결과; unrelated scope 삭제 없음 |
| 알 수 없는 kind/type | valid namespace면 저장·neighbors 가능; default impact 전파 없음 |
| shutdown/cancel | goroutine/reader 종료, 미완료 snapshot 미publish |
| synthetic sensitive attributes | allowlist로 배제, 로그/API/fixture output에서 marker 미검출 |

Provider contract runner는 third-party provider가 core DB 없이 projection을 검증할 수 있어야 한다. identity/query E2E는 동일 replay stream을 memory와 PostgreSQL에 적용해 비교한다.

## 3. Protocol/lifecycle scenarios

1. Upsert → 동일 seq retry: 한 번 적용, 동일 ACK.
2. 동일 seq 다른 payload: reject, graph 불변.
3. seq 1,3,2: 3은 gap reject; 2 후 3 retry만 적용.
4. new epoch 발급 후 old epoch write/heartbeat: 둘 다 거부.
5. snapshot begin → 일부 chunks → provider crash: 이전 live graph 유지, grace 이후 stale.
6. valid snapshot complete에서 이전 resource가 없음: 그 scope에 한해 tombstone.
7. snapshot chunk count/owner uniqueness 불일치: atomic rejection.
8. snapshot에 없는 edge: owner/scope 규칙에 따라 철회.
9. heartbeat 성공 + mount scope failure: network coverage만 유지, mount는 stale.
10. delete → delayed pre-delete event: seq/epoch로 거부. newer seq에 old source state가 실리지 않도록 provider keyed cache 테스트.
11. DB commit 전 crash: data/cursor 둘 다 없음. commit 후 ACK 전 crash: retry는 duplicate.
12. resolver enqueue 후 crash: restart 후 derived graph 수렴.
13. resolver 실행 중 input 변경: old evidence digest의 edge는 유효하지 않음; dirty generation 유지.
14. source clock 미래/과거 skew: ordering/state transition 불변.
15. receipt/tombstone retention 후 늦은 retry: 재적용 금지, 필요 시 resnapshot.
16. watch reconnect/relist 도중 queue overflow: partial view를 complete로 발행하지 않음.

## 4. Kubernetes-specific scenarios

- Deployment→ReplicaSet→Pod ownerReferences UID 일치. label이 같아도 owner UID가 다르면 OWNS 생성 금지.
- object 재생성으로 같은 이름/새 UID: old resource와 새 resource 분리, 지연 tombstone이 새 object를 삭제하지 않음.
- Pod `nodeName`은 name reference이므로 cache에서 현재 Node UID를 찾고 Pod observation 및 Node lookup revision을 provenance에 보존. Node 이름 재사용 시 기존 Pod의 UID-less nodeName만으로 새 OS 위치를 확정하지 않도록 pod/node 시간·현재관측 일관성 검증; 모호하면 pending/candidate. 이 API만으로 완전한 과거 배치를 복원할 수 없음을 명시.
- Container runtime ID 재시작, init/ephemeral/regular 구분, pending ID 없음, terminated status 잔존.
- namespace/kind scope RBAC forbidden: 해당 coverage 실패, unrelated namespace 삭제 금지.
- tombstone `DeletedFinalStateUnknown` 처리, informer relist 이후 재수렴.
- PVC `volumeName`과 PV `claimRef.uid` 대응. stale/rebound PV가 다른 PVC에 연결되지 않음.
- volumeDevices와 volumeMounts 구분; 미사용 volume 선언을 실제 OS mount로 주장하지 않음.
- Service selector change, empty selector, selectorless Service.
- EndpointSlice targetRef UID, targetRef 없음, ready=false, serving=true/terminating=true, dual-stack, 동일 Pod 중복 endpoint.
- endpoint IP 동일하다는 이유로 다른 namespace의 Pod에 연결하지 않음.

## 5. Linux-specific scenarios

- agent container identity 대신 injected host machine-id/boot-id를 읽는지 확인.
- reboot: OS identity 유지, process/cgroup/mount generations 변경.
- OS reinstall/enrollment state 유실/복제: 무조건 같은 hostname으로 합치지 않음.
- PID reuse와 stat starttime, process read 중 exit, hidepid/permission denied.
- host netns와 agent netns 구분, ifindex reuse, bridge/bond 중복 node 없음.
- IPv4/IPv6, 같은 IP가 다른 interface/netns에 존재, multipath route와 policy routing table 보존.
- mountinfo escaped path, bind mount, tmpfs, overlay, NFS, raw block, device-mapper, 다중 device, major:minor reuse.
- filesystem UUID 없음/복제: UUID만으로 전역 resource 합병 금지.
- cgroup v2 runtime path pattern, truncated ID, suffix 유사 ID, unsupported layout, inode/path 재사용.
- 권한이 부족한 DMI/sysfs/udev 항목은 unknown/partial이고 빈 snapshot success가 아님.

## 6. Correlation acceptance matrix

| 증거 | 기대 |
|---|---|
| unique machine-id와 UUID 모두 일치 | deterministic BACKED_BY |
| 유효한 machine-id만 공유, 양쪽 동일 kind에서 unique | 제안 정책상 match; ADR-005 승인 대상 |
| machine-id 일치, UUID 충돌 | trusted edge 없음, conflict candidate |
| OS 두 개에 동일 identifier | trusted edge 없음, ambiguous 후보 |
| hostname만 일치 | heuristic candidate만 |
| domain 다름 | match 없음 |
| empty/all-zero/placeholder UUID | matching에서 제외 |
| 기존 match evidence 교체/삭제 | 이전 derived assertion 즉시 invalid, 재계산/철회 |
| stale 중복 evidence | 단순 제외하여 잘못 unique로 만들지 않음 |
| Container full ID와 cgroup exact ID + 검증된 OS 일치 | BACKED_BY |
| Container ID prefix 또는 다른 OS의 동일 문자열 | trusted edge 없음 |
| CSI handle만 있고 Linux 매핑 없음 | PV/Linux edge 없음 |

## 7. Query correctness

- directed/undirected shortest path, cycle, self-edge, disconnected nodes, deterministic tie-break.
- Namespace/Cluster containment를 통한 impact 폭발 없음.
- Node failure의 potential impact에 해당 Pod/RS/Deployment/게시된 Service 포함; 다른 node의 unrelated Pod 제외.
- 같은 Deployment의 healthy replica는 outage로 표시하지 않음. ancestor workload는 potential impact로 집계.
- NIC 하나를 root로 impact 실행해 OS 전체 장애로 승격하지 않음.
- unknown relationship은 neighbors에는 보이고 profile-based traversal에서는 자동 사용되지 않음.
- stale frontier, pending edge, permission-denied scope와 정말 연결이 없는 경우를 응답에서 구별.
- max-depth/nodes/edges/time 각각 독립적으로 truncation 시험. unlimited all-path enumeration 없음.
- read snapshot 중 concurrent writer가 있어도 한 응답은 혼합 DB snapshot이 되지 않음.

## 8. Integration / E2E / performance gates

Level 1: fixture·property tests, 네트워크/DB 없이 실행. core state machine에는 seeded randomized duplicate/reorder/restart sequences를 적용하고 같은 authoritative end state로 수렴하는지 확인.

Level 2: ephemeral PostgreSQL로 memory repository contract와 결과 비교, transaction fault injection, schema migrations up from empty, index plan 검사. kind/k3d로 informer discovery와 object mutation/relist 검증. 여기서 host OS correlation의 현실성을 증명했다고 주장하지 않는다.

Level 3: 2 Linux VM nodes, 실제 지원 runtime/cgroup v2, 최소 권한 DaemonSet, workload+PVC 예제, fresh Helm install → automatic trace/impact. provider kill/restart, core restart, blocked collector, node replacement, identifier collision을 주입한다. external PostgreSQL mode와 dev PostgreSQL mode를 각각 검증한다. 설치/테스트에 VMware/OpenStack은 요구하지 않는다.

실행 가능한 릴리스에는 지원 Kubernetes minor, distro/kernel/runtime versions를 정확히 기록한다. 구현 시점에 library와 supported versions를 선택·고정한다. 현재 설계는 특정 최신 버전 지원을 약속하지 않는다.

성능 시험은 단계별 10/100 nodes, 1k/10k pods, node당 100/1k processes로 resource 수·assertion 수·변경률을 별도 기록한다. ingest lag, convergence time, query p50/p95, RSS, DB writes, disk, snapshot publish lock time을 보고한다. 초기 목표치는 관측 후 정하며 달성되지 않은 SLO를 README에 기재하지 않는다.

Prometheus metric labels는 provider type/collector/result 정도로 제한하고 resource ID/native path를 넣지 않는다. ingestion errors, queue backlog, stale count, resolution conflicts, query latency, snapshot failures를 기존 metrics client로 노출한다.

## 9. 공개 전 문서 gate

README에 자동으로 연결되는 path, 연결되지 않는 조건, 지원 환경, 최소 권한, potential-impact의 의미, DB retention, 인증/bootstrap 흐름을 명시한다. demo 영상보다 재현 가능한 command sequence와 expected output을 우선한다. fixture 통과와 real E2E 통과를 구분해서 발표한다.
