# Repository Guide

## Layout

- `apps/web`: Nuxt static public site.
- `apps/admin`: Vue content administration client.
- `apps/server`: Go content and publishing service.
- `packages/schema`: Canonical content schema and generated types.
- `content/fixtures`: Development-only sample content.
- `docs`: Architecture, implementation plans, and operations.

## Commands

- `pnpm dev`: start the public site locally.
- `pnpm test`: run JavaScript and Vue tests.
- `pnpm typecheck`: run all TypeScript checks.
- `pnpm lint`: run ESLint.
- `pnpm generate`: generate schema types and the static site.
- `pnpm verify`: run all public-site verification.
- `go test ./...`: run Go tests from `apps/server` once present.

## Contracts

- Treat `packages/schema/schema` as the canonical content contract.
- Do not hand-edit files named `generated.ts` or `generated.go`.
- Public pages must render from build-time snapshots without a runtime content API.
- Third-party music and social sites are link targets, not runtime data sources.

## Workflow

- Add a failing behavior test before production code.
- Keep modules focused and prefer explicit interfaces over provider-specific APIs.
- Run the narrow test first, then the affected package suite before committing.
