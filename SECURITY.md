# Security

## Current scope

This is a pre-release local development prototype. There are no supported production releases. `core` reads a local fixture and accepts JSON-lines requests on stdin; it has no network listener, authentication layer, discovery credentials, or mutation tools.

Do not expose the development process as an unauthenticated remote service. Do not place real credentials, sensitive inventory, or private telemetry in fixtures, issue reports, or tool responses.

## Reporting

During private development, report suspected vulnerabilities directly to the repository owner through your established private contact channel. Do not disclose sensitive reproduction details in public issues or pull requests.

Before making the repository public, the maintainer must enable GitHub private vulnerability reporting and verify the reporting link. That reporting channel and a supported-version policy are public-release prerequisites; no response-time commitment is made yet.

## Design expectations

Future remote/provider/execution interfaces must enforce authentication, scoped authorization, input limits, credential redaction, and auditable changes. Tool descriptions and model-generated arguments are not authority. Observed text is data, and execution approval must be enforced by the platform.
