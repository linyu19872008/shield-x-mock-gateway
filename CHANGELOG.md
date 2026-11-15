# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- Refactored the flat `cmd/mock-gateway` package into focused `internal/`
  packages (`contract`, `config`, `evaluator`, `template`, `generator`,
  `agentlink`, `state`, `logging`, `engine`, `control`, `dataplane`,
  `conformance`, `cli`, `version`). CLI behavior and public module path are
  unchanged.

### Added

- Bilingual control UI (English and Simplified Chinese) with browser detection,
  a sidebar switcher, cookie persistence, and `?lang=en|zh` overrides.
- Localized API error messages driven by control-API error codes.
- English and Chinese UI screenshots.

## [0.1.1] - 2026-11-15

### Added

- Standalone MIT-licensed repository.
- `gateway` data-plane command with mock 200/403/301/302/307 and challenge
  responses.
- Bilingual English and Simplified Chinese user documentation.
- Documentation link checking and open-source contribution templates.

### Fixed

- CLI entrypoint moved to `cmd/mock-gateway`, so
  `go install .../cmd/mock-gateway@v0.1.1` installs a binary named `mock-gateway`.
- Configuration example now reports the correct agent version.

### Changed

- Root README is English by default; Chinese is available at `README.zh-CN.md`.
- Internal milestone notes were removed from the public user documentation.

## [0.1.0] - 2026-11-15

**Retracted.** This tag used the pre-`cmd/mock-gateway` module layout and
installed a binary with the wrong name. Use `0.1.1` instead. The Go proxy cannot
rewrite the immutable tag, so the module declares `retract v0.1.0`.
