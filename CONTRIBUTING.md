# Contributing

Opsplatform is an early prototype. Start with the README and current product direction before implementing a subsystem. Larger changes to identity, lifecycle, query semantics, providers, or execution should begin with an issue or design proposal.

## Local workflow

1. Fork and clone the repository, then create a focused branch from `main`: `feat/<topic>`, `fix/<topic>`, `docs/<topic>`, or `chore/<topic>`.
2. Use Go 1.25 or later. There are currently no third-party Go dependencies.
3. Keep behavior in `internal/operations`; CLI and tool adapters should call that service.
4. Test meaningful behavior, especially identity collisions, partial observations, bounded traversal, and unsafe impact propagation.
5. Run `gofmt -w cmd internal pkg`, `make check`, and `make build`.
6. Open a pull request against `main`, describing the problem, resulting behavior, and validation. Maintainers squash merge after required checks pass.

Tests live next to the package they exercise. Fixture tests must not require a cluster, host `/proc`, external credentials, or a running database. Use synthetic, sanitized data. Never add credentials, machine inventories, or private customer identifiers.

Do not silently turn a name match into identity, membership into dependency, or potential impact into a confirmed outage. Preserve provenance and visibility limitations in query responses.

New provider kinds must remain open strings. Provider-specific collection code belongs outside core operations. Do not add an LLM SDK or MCP requirement to the core.

## Scope and compatibility

The current implementation is M1: fixture-backed graph operations and adapters. Provider ordering/coverage, live collectors, persistence, diagnosis, and execution are separate stages. Package and tool APIs are not yet stable.

The Korean architecture documents include historical proposals. Check the documentation index for precedence before following a proposal literally. English summaries and documentation improvements are welcome.

See [branch and release policy](docs/development.md) for merge and versioning conventions.

## Licensing

Submit only code and fixtures you have the right to contribute. Contributions are provided under this repository's Apache-2.0 license. Keep applicable third-party notices when introducing reused material, and discuss new dependencies in the pull request.
