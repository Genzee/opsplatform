# Security

## Current scope

This is a pre-release local development prototype. There are no supported production releases. `core` reads a local fixture and accepts JSON-lines requests on stdin; it has no network listener, authentication layer, discovery credentials, or mutation tools.

Do not expose the development process as an unauthenticated remote service. Do not place real credentials, sensitive inventory, or private telemetry in fixtures, issue reports, or tool responses.

## Reporting

Use [GitHub private vulnerability reporting](https://github.com/Genzee/opsplatform/security/advisories/new) to report suspected vulnerabilities. Private reporting is enabled. Do not disclose sensitive reproduction details in public issues or pull requests.

Include the affected commit, a minimal sanitized reproduction, and the expected impact. There are no supported production releases or response-time commitments yet. A supported-version policy will be added before the first production release.

## Design expectations

Future remote/provider/execution interfaces must enforce authentication, scoped authorization, input limits, credential redaction, and auditable changes. Tool descriptions and model-generated arguments are not authority. Observed text is data, and execution approval must be enforced by the platform.
