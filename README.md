# leafrun-infra

## Make the services runnable

This planned repository will hold deployment and operations files for the services: Kubernetes setup, databases, object storage, NATS, Temporal, Redis, Ory, and observability. It should provide a repeatable development environment before production scaling.

Each service keeps its own application code and migrations. Infra wires those pieces together without owning document or access rules.

**Status:** this repository is planned and has no runnable implementation yet. For code you can try now, see the [local prototypes](https://github.com/leafrun/leafrun/blob/main/docs/repositories.md).

**First roadmap step:** [LEA-18 — Bring up object storage](https://linear.app/bulnik/issue/LEA-18/e03-s3-rezultaty-i-ochistka). Linear may require workspace access; the public [roadmap](https://github.com/leafrun/leafrun/blob/main/docs/roadmap.md) explains the build order. See the [repository map](https://github.com/leafrun/leafrun/blob/main/docs/repositories.md) and [contribution guide](https://github.com/leafrun/leafrun/blob/main/CONTRIBUTING.md).

No project license has been selected yet.
