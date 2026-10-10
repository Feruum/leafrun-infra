# leafrun-infra

Planned infrastructure for Leafrun: Kubernetes, PostgreSQL, S3, Redis, NATS, Temporal and Ory.

**Status: planned component.** This repository currently contains its scope and roadmap links. It does not yet provide a runnable service or package. The existing local rendering prototype lives in [leafrun-generation](https://github.com/leafrun/leafrun-generation).

## Responsibility

Kubernetes configuration, ingress routes, Redis, NATS, PostgreSQL, S3 configuration, Temporal, Ory, secret store, observability.

## Boundaries

This repository owns deployment configuration and operational tooling, not domain business logic. Secrets must be supplied by the deployment environment.

## Roadmap

- [LEA-18 — S3 artifacts and garbage collection](https://linear.app/bulnik/issue/LEA-18/e03-s3-rezultaty-i-ochistka)
- [LEA-25 — Reliable asynchronous server rendering](https://linear.app/bulnik/issue/LEA-25/e04-nadyozhnyj-asinhronnyj-servernyj-render)
- [LEA-82 — Live collaboration and project ownership](https://linear.app/bulnik/issue/LEA-82/e11-live-collaboration-i-vladelec-proekta)
- [LEA-114 — Caching, operations and scaling](https://linear.app/bulnik/issue/LEA-114/e15-keshirovanie-ekspluataciya-i-masshtabirovanie)

See the [development roadmap](https://github.com/leafrun/leafrun/blob/main/docs/roadmap.md) for dependencies and acceptance milestones. Creating this repository does not complete its implementation issues.

## Contributing

Use English for code comments, documentation, examples, commit messages and pull requests. See the [contribution guide](https://github.com/leafrun/leafrun/blob/main/CONTRIBUTING.md) and [current architecture](https://github.com/leafrun/leafrun/blob/main/docs/architecture.md).

## License status

A project license has not been selected. No license is assigned by this repository scaffold.
