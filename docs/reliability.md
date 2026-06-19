# Reliability and consistency contract

SPACK treats asset delivery as a protocol contract, not only as implementation unit tests.

## Default CI gates

- `task lint`, `task check`, `task test`, and `task build` run on Ubuntu and Windows.
- `task test:race` runs on Linux CI.
- Fuzz targets are committed as normal Go fuzz tests, so their seed corpus runs during `go test ./...`.
- `task integration:container:smoke` runs on Linux CI against the built container image.

## Protocol coverage

The HTTP matrix covers:

- `GET` and `HEAD`.
- `ETag` and `If-None-Match`.
- `Last-Modified` and `If-Modified-Since`.
- `Range` partial content.
- `Range` plus `Accept-Encoding`, where Range requests must bypass encoded variants and preserve partial response headers.

Focused fuzz targets cover:

- request path normalization, including encoded traversal, backslashes, and UNC-like input.
- `Accept-Encoding` parsing.
- image `Accept` parsing.
- Range header handling at the HTTP boundary.

Run extended fuzzing with:

```sh
task test:fuzz:parsers
```

## Source and cache consistency

Source rescan tests assert that file changes, deletes, and renames reconcile:

- catalog entries.
- source sidecar variants.
- generated artifact variants.
- memory cache entries.
- artifact files on disk.

The consistency rule is strict: a stale source asset must not keep a stale catalog record, memory cache entry, or generated artifact.

## Bounded pipeline queue contract

The lazy generation pipeline uses a bounded queue intentionally.

- Duplicate pending requests are deduplicated.
- When the queue is full, the new enqueue request is dropped.
- Dropping a generation request must not fail the client request; the current response continues to serve the origin asset or an existing usable artifact.
- Dropped requests are not retained in the pending set, so a later request can enqueue the same generation work again.
- SPACK does not perform an internal retry loop for full-queue drops; retry is request-driven.
- Operators must alert on sustained `pipeline_enqueue_dropped_total` growth with high `pipeline_queue_length / pipeline_queue_capacity`.

Relevant metrics:

- `pipeline_queue_length`
- `pipeline_queue_capacity`
- `pipeline_enqueue_dropped_total`
- `pipeline_enqueue_deduplicated_total`

## Container black-box contract

The container smoke test starts the actual image with:

- read-only root filesystem.
- all Linux capabilities dropped.
- `no-new-privileges`.
- read-only asset volume.
- writable `/tmp` tmpfs for runtime artifact cache.

It verifies `/livez`, `/`, `/app.js`, and a Range request against the running container.

## Catalog stress testing

Large catalog scans are opt-in because they create many files.

```sh
task test:stress:catalog
SPACK_STRESS_ASSET_COUNT=1000000 task test:stress:catalog
```

The stress test records asset count, scan duration, source bytes, and heap deltas in test logs. Release qualification should persist those logs with the tested machine shape and image digest.
