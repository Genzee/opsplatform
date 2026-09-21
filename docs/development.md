# Branch and release policy

## Branches

- `main` is the default integration branch and should remain buildable with passing tests.
- Create short-lived `feat/*`, `fix/*`, `docs/*`, or `chore/*` branches from current `main`.
- Submit pull requests to `main`; do not push development changes directly to it.
- There is no permanent `develop` branch or separate integration tree in this early project.
- Delete merged topic branches. Do not force-push or delete `main`.

## Pull requests and checks

CI checks formatting, race-enabled tests, `go vet`, builds, and fixture CLI/tool smoke tests on Go 1.25.x and the current stable Go release. Workflow dependencies are pinned to commit SHAs and updated through Dependabot.

The intended branch rules require a PR, both CI checks, resolved review conversations, an up-to-date branch, and linear history. Squash merge is the only enabled merge method.

During the single-maintainer bootstrap, the required number of approving reviewers is zero so the maintainer can merge their own PR after CI. This does not bypass PRs or checks. Increase it to one independent reviewer when a second active maintainer is available. A CODEOWNERS file requiring the sole maintainer's self-approval is intentionally not introduced.

Use clear imperative commit/PR subjects; Conventional Commit prefixes such as `feat:`, `fix:`, `docs:`, and `chore:` are encouraged. The squashed PR title becomes the main-branch commit subject. Do not commit credentials or generated binaries.

GitHub feature availability can limit server-side protection for a private repository. The maintainer must verify enforcement in repository settings; a written policy alone is not protection. Any unavailable rule should be documented rather than silently claiming it is enforced.

**Verified enforcement status (2026-09-21):** the repository is public. Branch protection is enabled on `main`, including for administrators. PRs, successful `Go 1.25.x` and `Go stable` checks from GitHub Actions, up-to-date branches, resolved review conversations, and linear history are required. Force pushes and deletion are disabled. Squash-only merging and automatic deletion of merged branches are enabled. Required approving reviewers remain zero during the single-maintainer bootstrap.

## Releases

- Tag a tested commit on `main` with an annotated semantic version, such as `v0.1.0-alpha.1` or `v0.1.0`.
- Publish release notes describing behavior, compatibility changes, validation, and supported environments.
- Do not move or overwrite a released tag.
- Do not tag the fixture prototype as a completed live-discovery release.
- Add `release/v0.x` maintenance branches only when a released line actually needs separate support. Apply fixes to `main` first where practical, then backport with a PR.

No release has been published yet. Tag protection and artifact/signing workflows should be configured when the first release pipeline is introduced.
