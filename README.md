# Opsplatform

An infrastructure operations platform for people and software agents.

**Status: early prototype.** Opsplatform is a working name. The current implementation explores synthetic infrastructure data; it does not discover or operate live systems yet.

The goal is to connect application workloads to their real network and storage dependencies, explain problems using evidence, and eventually plan, provision, and verify infrastructure changes. The resource graph is the foundation, not the whole product.

## What works today

- Concrete resources identified by provider, kind, and native incarnation ID.
- Provider-owned relationship assertions with provenance and unresolved references.
- Resource search, neighbors, infrastructure and dependency traversal, potential impact, and shortest paths.
- Lifecycle filtering, stale-data warnings, and bounded traversal.
- A CLI and eight vendor-neutral function tools calling the same application service.
- An in-memory repository and a synthetic shared-storage example.

No OpenAPI, MCP, model SDK, or third-party Go module is required. An external agent can register the tool definitions and dispatch calls to the handlers. See the [tool adapter guide](docs/architecture/tool-adapter.md).

## Quick start

Requires Go 1.25 or later. Run from the repository root:

```sh
make check
make build

./bin/infra -fixture examples/shared-storage.json trace payment-api
./bin/infra -fixture examples/shared-storage.json impact storage-01
./bin/infra -fixture examples/shared-storage.json path payment-api storage-01
./bin/core -fixture examples/shared-storage.json < examples/tools.jsonl
```

The fixture contains two applications on different nodes that share `storage-01`. Its potential impact includes `payment-api` and `report-api`, but excludes `unrelated-api` in the same namespace. These are synthetic facts, not live discoveries or a root-cause diagnosis.

All CLI flags precede the command. Output is JSON. Run `./bin/infra -h` for options.

If Go is not installed, an existing Go Docker image can run the checks:

```sh
docker run --rm --network none -v "$PWD:/work" -w /work golang:1.25 go test -race ./...
docker run --rm --network none -v "$PWD:/work" -w /work golang:1.25 go run ./cmd/infra -fixture examples/shared-storage.json impact storage-01
```

These commands require the image to be present already. They do not download Go dependencies or access infrastructure.

## Architecture

```text
CLI --------------------+
                        |
External agent -> tools -+-> shared operations -> repository -> memory
```

The application service owns query semantics. Adapters translate calls; they do not implement separate graph logic. Future remote adapters, discovery providers, PostgreSQL persistence, diagnosis, and execution workers will build on these boundaries.

```text
cmd/
  core/                    Local JSON-lines tool process
  infra/                   CLI
pkg/model/                 Resource, assertion, provenance, lifecycle
internal/
  operations/              Shared service and traversal profiles
  repository/              In-memory storage
  adapters/tools/          Tool catalog, validation, dispatch, stdio
  fixture/                 Strict fixture loading
examples/                  Synthetic topology and tool requests
docs/                      Architecture proposals and ADR drafts
.github/                   CI and contribution templates
```

## Current boundaries

`core` is a local stdin/stdout development process, not a network server or production daemon. State is lost at process exit. IDs are stable within a process, but a new process assigns new IDs. A tool client must use IDs returned by the same running core. Each CLI invocation reloads the fixture; use names and filters across separate invocations.

There is no live discovery, provider authentication or ordering protocol, automatic identity resolver, coverage tracking, PostgreSQL backend, diagnosis engine, provisioning, or mutation tool yet. Cross-provider edges in the example are predeclared fixture facts. The tool catalog advertises only implemented operations.

The memory repository copies snapshots and is limited to 20,000 resources, 50,000 assertions, and 4 MiB per fixture/batch. Production persistence will require indexed read views. Traversal profiles are currently built in; external profile loading is not implemented.

Attributes and provenance are data, not instructions. Do not use real secrets or credentials in fixtures.

## Development and roadmap

1. **Current:** in-memory graph, shared operations, CLI, function tools, contract tests.
2. Provider protocol, lifecycle, collection coverage, and restart reconciliation.
3. Kubernetes and Linux discovery, followed by cross-provider identity resolution.
4. A complete network/storage dependency path and evidence-based diagnosis.
5. Persistent runtime, packaging, and reproducible deployment.
6. Planned and authorized provisioning/operational changes, followed by observation.

Future stages remain proposals, not implemented features or frozen APIs. See the [documentation index](docs/README.md) for design history, current decisions, and remaining questions. The initial design notes are in Korean.

Contributions are welcome. Start with [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and the [code of conduct](CODE_OF_CONDUCT.md). Development uses protected `main`, short-lived topic branches, pull requests, required CI, and squash merges. See the [branch and release policy](docs/development.md).

## License

[Apache License 2.0](LICENSE).
