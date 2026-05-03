# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-05-03

### Changed
- Default index changed from `SPX` to `us-tradable`
- Upgrade pvbt dependency to v0.8.2

## [0.1.4] - 2026-05-01

### Changed
- Upgrade pvbt dependency to v0.8.1

## [0.1.3] - 2026-04-25

### Changed
- Upgrade pvbt dependency to v0.8.0
- Regenerate testdata snapshot for pvbt's v5 snapshot schema
- Accept either META or its predecessor FB ticker in the high-momentum smoke test

## [0.1.2] - 2026-04-23

### Changed
- Upgrade pvbt dependency to v0.7.7

## [0.1.1] - 2026-04-21

### Fixed
- Remove local pvbt replace directive so the module resolves correctly outside the monorepo



## [0.1.0] - 2026-04-21

### Added
- Initial release of Momentum Factor strategy
- Implements Momentum Factor (12-1) strategy for ranking assets by trailing 12-month return minus most recent month
- Ginkgo unit tests with snapshot-based test coverage

[0.1.0]: https://github.com/penny-vault/momentum-factor/releases/tag/v0.1.0

[0.1.1]: https://github.com/penny-vault/momentum-factor/compare/v0.1.0...v0.1.1
[0.1.2]: https://github.com/penny-vault/momentum-factor/compare/v0.1.1...v0.1.2
[0.1.3]: https://github.com/penny-vault/momentum-factor/compare/v0.1.2...v0.1.3
[0.1.4]: https://github.com/penny-vault/momentum-factor/compare/v0.1.3...v0.1.4
[0.2.0]: https://github.com/penny-vault/momentum-factor/compare/v0.1.4...v0.2.0
