# Documentation

## Current implementation

The root [README](../README.md) is authoritative for implemented behavior and limitations. M1 provides a shared Go operations service, a CLI, and a function-tool adapter over synthetic in-memory data.

- [Product direction](architecture/product-direction.md): people and external agents use common operations; provisioning remains a future core objective.
- [Tool adapter](architecture/tool-adapter.md): direct function dispatch and the local JSON-lines protocol. No OpenAPI or MCP requirement.
- [Branch and release policy](development.md): contributor workflow, CI, merges, and version tags.
- [Runtime, build, and deployment overview](runtime-overview.md): a Korean walkthrough of Go binaries, today's local processes, and the future deployment model.

## Design proposals and history

These documents are mostly Korean working notes and may describe future interfaces that are not implemented:

- [Initial architecture review](architecture/design-v0.1.md)
- [Initial Go, provider protocol, REST, and SQL proposals](architecture/contracts-v0.1.md)
- [Provider and system test contracts](architecture/test-contracts.md)
- [ADR-001 through ADR-010 drafts](adr/README.md)

`InfraGraph` was the initial working name. Mentions of it and its proposed namespaces in historical drafts are not current package names. The repository now uses the temporary project name Opsplatform and module path `github.com/Genzee/opsplatform`.

The initial transport-first/API proposal was superseded by a shared operations service with CLI and function-tool adapters. REST, gRPC, or MCP may be added as adapters when justified; none is required for the current tool interface. The initial topology-only product framing was expanded to diagnosis and eventual provisioning.

The implementation-start approval covered the first shared-service/tool-adapter foundation. Detailed provider, security, persistence, and execution proposals still need review at their implementation stages.
