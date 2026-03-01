# Architecture Dependency Order

## Layer Order

`cmd` -> `api` -> `app` -> `domain` -> `infra/pkg`

- `cmd`: composition root only (boot, wire, lifecycle).
- `api`: protocol adapters (HTTP route, request decode, response encode).
- `app`: use-case orchestration and transaction boundary.
- `domain`: business rules, domain types, service/repository contracts.
- `infra/pkg`: concrete adapters for DB/cache/network/provider/runtime.

## Mandatory Rules

1. Import direction must follow outer-to-inner only.
2. `pkg/**` must not import `api/**`.
3. Route declarations must live in `api/routes`; `cmd` only invokes registration.
4. Root feature packages must stay facade-oriented; implementations live in dedicated subpackages.
5. Migration must be single-track. When a new path is adopted, remove old code path immediately.

## Current Enforcement

- Script: `scripts/arch_guard.ps1`
- Make target: `make arch-guard`
- CI strict target: `make arch-guard-ci`

The guard currently checks:
- forbidden imports (`pkg` importing `api`)
- oversized production package warnings
- single-file production package warnings

CI strict mode (`ARCH_GUARD_STRICT=1`) upgrades warnings to errors.
