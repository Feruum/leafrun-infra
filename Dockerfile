# syntax=docker/dockerfile:1.7
FROM golang:1.27.0-bookworm@sha256:ded31c68586d2e49e760acc2e65a884b23d032e9bbbed0ae0c55abd3fcaf4452 AS build
WORKDIR /src
COPY --from=contracts . ./contracts
COPY --from=core . ./core
COPY --from=integrations . ./integrations
COPY --from=platform . ./platform
COPY . ./infra
RUN printf 'go 1.27.0\nuse (\n ./infra\n ./contracts\n ./core\n ./integrations\n ./platform\n)\nreplace github.com/leafrun/leafrun-contracts v0.1.0 => ./contracts\nreplace github.com/leafrun/leafrun-document-core v0.1.0 => ./core\nreplace github.com/leafrun/leafrun-integrations v0.1.0 => ./integrations\nreplace github.com/leafrun/leafrun-platform v0.1.0 => ./platform\n' > go.work
WORKDIR /src/infra
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -o /out/owner-fixture ./cmd/owner-fixture && CGO_ENABLED=0 go build -o /out/owner-router ./cmd/owner-router && CGO_ENABLED=0 go build -o /out/jetstream-bootstrap ./cmd/jetstream-bootstrap
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -o /out/dev-consumer ./cmd/dev-consumer
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS fixture
COPY --from=build /out/owner-fixture /owner-fixture
ENTRYPOINT ["/owner-fixture"]
FROM fixture AS owner-router
COPY --from=build /out/owner-router /owner-router
ENTRYPOINT ["/owner-router"]
FROM fixture AS jetstream-bootstrap
COPY --from=build /out/jetstream-bootstrap /jetstream-bootstrap
ENTRYPOINT ["/jetstream-bootstrap"]
FROM fixture AS dev-consumer
COPY --from=build /out/dev-consumer /dev-consumer
ENTRYPOINT ["/dev-consumer"]
