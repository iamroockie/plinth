# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-09-25

### Added

- `Healthz` handler for liveness probes.
- `Readyz` handler for readiness probes. It runs named `CheckFunc`s concurrently
  and answers 503 if any of them fails, panics or exceeds the timeout. It answers
  within the timeout even if a check ignores its context.
- `CheckFunc`, a readiness check. Methods such as `(*pgxpool.Pool).Ping` and
  `(*sql.DB).PingContext` can be used as is.
- `Map`, a shorthand for JSON objects in responses.

## [0.1.0] - 2026-09-24

Initial release.

[Unreleased]: https://github.com/iamroockie/plinth/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/iamroockie/plinth/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/iamroockie/plinth/releases/tag/v0.1.0
