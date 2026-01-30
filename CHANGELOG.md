# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Build with up-to-date pipelines.
- Resolve updated code linter findings.
- Replace `Chart.Version` with `Chart.AppVersion` in labels and image tag defaults.

## [0.12.3] - 2025-01-31

## [0.12.2] - 2024-10-24

### Changed

- Updated golang to v1.23.2

## [0.12.1] - 2024-10-23

### Added

- Added enable/disable switch on `tbot` components

## [0.12.0] - 2024-10-16

### Changed
- Changed the logic of setting roles by parsing `"%s-teleport-kube-agent-user-values"` configmaps to check if apps are enabled.
- Deprecated `MC-Namespace` and `tokenRoles`

## [0.11.2] - 2024-10-11

### Fixed
- Implemented `MC-Namespace` flag to avoid enabling roles except `kube` to workload clusters
- Increased `grpc buffer` size

## [0.11.1] - 2024-10-11

### Fixed

- Set only `kube` by default to aviod issues when no apps are set.

## [0.11.0] - 2024-10-11

### Changed

- Implemented option to generate combined tokens with multiple roles
- Change ownership to Team Shield

### Fixed

- Disable logger development mode to avoid panicking
- Chart to use `.Release.Namespace` namespace

## [0.10.0] - 2024-08-19

### Added

- Added tbot feature flag, enabled with `--tbot` flag
- If tbot feature flag is set, creates configmap and append to tbot app extra config for generating  kubeconfig.

## [0.9.3] - 2024-05-08

### Changed

- Increase token validity for node and kube agents

## [0.9.2] - 2024-05-07

### Added

- Added affinity and tolerations for operators' deployments to run on control-plane

## [0.9.1] - 2024-04-01

### Fixed

- Use PodMonitor for monitoring instead of legacy system.

## [0.9.0] - 2024-03-21

### Changed

- Updated `tbot` deployment to use standalone `tbot` image (smaller image size)
- Bump teleport version to `15.1.7`

## [0.8.4] - 2024-01-04

### Changed

- Updated cilium network policy for tbot and teleport-operator

## [0.8.3] - 2024-01-04

### Added

- Set `TELEPORT_TLS_ROUTING_CONN_UPGRADE` environment variable

## [0.8.2] - 2024-01-04

### Added

- Add network policy for tbot
- Fixes CVE-2023-48795 in crypto package

## [0.8.1] - 2023-12-20

### Fixed

- Correct `CiliumNetworkPolicy` spec.

## [0.8.0] - 2023-12-19

### Added

- Add `CiliumNetworkPolicy` (disabled by default).

### Changed

- Configure `gsoci.azurecr.io` as the default container image registry.
- Correct path in `.gitignore`.

### Changed

- Remove CircleCI push to Vintage (aws-app-collection)

## [0.7.0] - 2023-11-28

### Changed

- Replace `-bot` suffix with `bot-` rpefix in tbot token name.

## [0.6.0] - 2023-11-21

### Added

- Adds support for Teleport Machine ID Bot for short-lived certificate for Teleport Cluster access.

### Fixed

- Fixes broken architecture diagram in README

## [0.5.0] - 2023-10-31

### Added

- Add push to CAPZ, CAPG, CAPV, CAPVCD app collection

## [0.4.0] - 2023-10-19

### Changed

- Adjust security context

### Fixed

- x/net@v0.14.0 - CVE-2023-39325

## [0.3.0] - 2023-09-28

### Changed

- Update deployment to be PSS compliant and PSP toggle.

## [0.2.1] - 2023-09-21

### Added

- Update README
- Tests

## [0.2.0] - 2023-08-15

### Fixed

- Leverage app platform for deploying teleport-kube-agent app

## [0.1.0] - 2023-08-09

[Unreleased]: https://github.com/giantswarm/teleport-operator/compare/v0.12.3...HEAD
[0.12.3]: https://github.com/giantswarm/teleport-operator/compare/v0.12.2...v0.12.3
[0.12.2]: https://github.com/giantswarm/teleport-operator/compare/v0.12.1...v0.12.2
[0.12.1]: https://github.com/giantswarm/teleport-operator/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/giantswarm/teleport-operator/compare/v0.11.2...v0.12.0
[0.11.2]: https://github.com/giantswarm/teleport-operator/compare/v0.11.1...v0.11.2
[0.11.1]: https://github.com/giantswarm/teleport-operator/compare/v0.11.0...v0.11.1
[0.11.0]: https://github.com/giantswarm/teleport-operator/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/giantswarm/teleport-operator/compare/v0.9.3...v0.10.0
[0.9.3]: https://github.com/giantswarm/teleport-operator/compare/v0.9.2...v0.9.3
[0.9.2]: https://github.com/giantswarm/teleport-operator/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/giantswarm/teleport-operator/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/giantswarm/teleport-operator/compare/v0.8.4...v0.9.0
[0.8.4]: https://github.com/giantswarm/teleport-operator/compare/v0.8.3...v0.8.4
[0.8.3]: https://github.com/giantswarm/teleport-operator/compare/v0.8.2...v0.8.3
[0.8.2]: https://github.com/giantswarm/teleport-operator/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/giantswarm/teleport-operator/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/giantswarm/teleport-operator/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/giantswarm/teleport-operator/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/giantswarm/teleport-operator/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/giantswarm/teleport-operator/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/giantswarm/teleport-operator/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/giantswarm/teleport-operator/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/giantswarm/teleport-operator/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/giantswarm/teleport-operator/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/giantswarm/teleport-operator/releases/tag/v0.1.0
