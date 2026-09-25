# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-09-25

### Changed

- **Breaking:** `details` of a 422 `validation_error` response is now an ordered
  array of violations instead of an object:
  `[{"field": "interval_seconds", "code": "out_of_range", "params": {"min": 10, "max": 86400}}]`.
  A violation can carry parameters, a field can have several violations, and a
  violation that is not about one field has `"field": ""`. Without violations the
  response has no `details`.
- **Breaking:** `ValidationError` takes `...FieldViolation` instead of
  `ErrorDetails`, and `ErrorDetails` is removed. To migrate, replace
  `ValidationError(ErrorDetails{"f": "c"})` with
  `ValidationError(FieldViolation{Field: "f", Code: "c"})`. Each key of a
  multi-key `ErrorDetails` becomes a separate `FieldViolation`; `details` keeps
  the order in which they are passed.
- **Breaking:** `plinthtest.Error.Details` is now `[]plinth.FieldViolation`.

### Added

- `FieldViolation`, an element of `details` with `Field`, `Code` and optional
  `Params`.
- `MatchViolations` and `FieldRule`, which map sentinel errors to violations with
  `errors.Is`. They work with `errors.Join` and wrapped errors and return every
  match in the order of the rules.

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

[Unreleased]: https://github.com/iamroockie/plinth/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/iamroockie/plinth/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/iamroockie/plinth/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/iamroockie/plinth/releases/tag/v0.1.0
