# marchiobj

Educational, S3-inspired **single-node object store** in Go.

Opaque bytes plus a small metadata index (bucket/key → blob id, size, etag).
PUT is an immutable replace: write a blob, then atomically swap metadata.
JSON HTTP, not AWS XML.

**Not MinIO / not AWS.** One disk, no IAM, no erasure coding.

See [PLAN.md](PLAN.md) for the MVP slices.

---

## Quick start

**Go (1.22+):**

```bash
go test ./...
go run ./cmd/marchiobj -dataDir=./data -addr=:7300
```

**Docker:**

```bash
docker build -t marchiobj .
docker run --rm -p 7300:7300 -v "$PWD/data:/data" marchiobj
```

**Health + create a bucket:**

```bash
curl -s localhost:7300/healthz
curl -s -X PUT localhost:7300/buckets/logs
curl -s localhost:7300/buckets
```

Flags: `-addr=:7300`, `-dataDir`.
