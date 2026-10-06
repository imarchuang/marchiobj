# marchiobj

Educational, S3-inspired **single-node object store** in Go.

Opaque bytes plus a small metadata index (bucket/key → blob id, size, etag).
PUT is an immutable replace: write a blob, then atomically swap metadata.
JSON HTTP, not AWS XML.

**Not MinIO / not AWS.** One disk, no IAM, no erasure coding.

See [PLAN.md](PLAN.md) for the MVP slices and [OBJECTSTORE.md](OBJECTSTORE.md)
for layout, atomic PUT, Range GET, and multipart complete.

---

## Quick start

**Go (1.22+):**

```bash
go test ./...
go run ./cmd/marchiobj -dataDir=./data -addr=:7300
```

**Docker:**

```bash
docker compose up --build
# listens on http://localhost:7300
```

**Demo:**

```bash
curl -s localhost:7300/healthz
curl -s -X PUT localhost:7300/buckets/logs
curl -s -X PUT localhost:7300/buckets/logs/objects/2026/a.txt -d hello
curl -s localhost:7300/buckets/logs/objects/2026/a.txt
curl -s 'localhost:7300/buckets/logs/objects?prefix=2026/'
curl -s -D- -o /dev/null -H 'Range: bytes=0-4' localhost:7300/buckets/logs/objects/2026/a.txt
```

Flags: `-addr=:7300`, `-dataDir`.
