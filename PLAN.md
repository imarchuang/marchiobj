# marchiobj — S3-inspired object store MVP

Educational blob store in Go. DDIA-adjacent: **control plane vs data plane**,
immutable objects, metadata index, multipart — not IAM, not erasure coding.

**Not MinIO / not AWS.** HTTP subset of S3-style verbs on one disk.

Optional later: CDC-style “derived data” is **not** this repo; wire
marchisql → marchiq → marchindex in a compose demo when those are ready.

---

## Learning goal

1. Object store = **opaque bytes + small metadata** (bucket/key → blob id,
   size, etag). Listing never scans blob files.
2. **PUT is immutable replace:** new blob id, then atomic metadata swap;
   old blob GC later (or immediately if unreferenced).
3. **ETag** (md5 or sha256 hex) for If-Match / integrity.
4. **Range GET** without loading the whole object.
5. **Multipart:** upload parts to a temp area, complete concatenates
   (or links) into one blob, then publishes metadata.

**Pass bar:** PUT 10 MB, GET with `Range: bytes=0-99`, LIST prefix, multipart
complete; crash during complete does not leave a visible half object.

---

## Concepts we keep (and drop)

| S3 | marchiobj v0 | Deferred |
|---|---|---|
| Bucket | directory + `bucket.json` | versioning, lifecycle |
| Key | UTF-8 path-like string | directory placeholders |
| PUT/GET/DELETE/HEAD | HTTP | CopyObject, tagging |
| ETag | sha256 hex | MD5 compatibility |
| LIST | prefix + delimiter `/`, max-keys | pagination token proper |
| Multipart | init / part / complete / abort | extra checksums per part |
| Data files | `blobs/{id}` content-addressed or uuid | erasure, encryption |
| Metadata | `meta.sqlite` or JSON files per key | Dynamo-style metadata cluster |

**Non-goals:** IAM, presigned URLs, website hosting, replication, S3 XML
fidelity (JSON is fine; optional XML later).

---

## Core loop

```text
PUT /bkt/k
  write tmp → fsync → rename blobs/{blobId}
  write metadata {key, blobId, size, etag, mtime} atomically
  unlink previous blobId if unused

GET /bkt/k
  lookup meta → open blob → Range if requested

Multipart complete
  write new blob (concat parts) in tmp, rename, then meta swap
  readers never see incomplete concat
```

---

## On-disk layout

```text
{dataDir}/
  buckets/
    {bucket}/
      bucket.json
      meta/
        keys.jsonl or per-key json   # key → blobId, size, etag
      blobs/
        {blobId}
      multipart/
        {uploadId}/
          meta.json
          part-00001
          part-00002
```

Prefer **content-addressed blobId = sha256** so identical PUT is a no-op
on data (metadata still updates mtime). Collision = same content.

---

## API (JSON, S3-shaped paths)

| Method | Path | Purpose |
|---|---|---|
| PUT | `/buckets/{bucket}` | create bucket |
| GET | `/buckets` | list buckets |
| PUT | `/buckets/{bucket}/objects/{key}` | PUT object (body = bytes) |
| GET | `/buckets/{bucket}/objects/{key}` | GET; honor `Range` |
| HEAD | same | size, etag, no body |
| DELETE | same | remove meta; GC blob |
| GET | `/buckets/{bucket}/objects?prefix=&delimiter=` | LIST |
| POST | `/buckets/{bucket}/uploads?key=` | initiate multipart |
| PUT | `/uploads/{id}/parts/{n}` | upload part |
| POST | `/uploads/{id}/complete` | publish object |
| DELETE | `/uploads/{id}` | abort |

Flags: `-addr=:7300`, `-dataDir`.

---

## MVP slices

### Slice 0 — skeleton
Healthz + create bucket.

### Slice 1 — PUT/GET/HEAD/DELETE
Single blob file + JSON meta; etag on HEAD.
Test: PUT, GET bytes equal, DELETE → 404.

### Slice 2 — LIST prefix/delimiter
`photos/a.jpg`, `photos/b.jpg`, `docs/x` → prefix `photos/` returns two.
Test: delimiter yields common prefixes.

### Slice 3 — Range GET
`Range: bytes=0-4` → 206, `Content-Range`.
Test: out-of-range 416.

### Slice 4 — multipart + atomic complete
Parts ≥ 1 (small for tests); complete is rename-then-meta.
Test: abort removes parts; kill during concat → object not listed.

### Slice 5 — polish
If-Match / If-None-Match, docker demo, `OBJECTSTORE.md`.

---

## Demo (graduation)

```bash
curl -X PUT localhost:7300/buckets/logs
curl -X PUT localhost:7300/buckets/logs/objects/2026/a.txt -d hello
curl localhost:7300/buckets/logs/objects/2026/a.txt
curl localhost:7300/buckets/logs/objects?prefix=2026/
```

---

## Relation to siblings

| Project | Overlap |
|---|---|
| **marchilogs** | immutable files + atomic rename; here the *key* is user path, not time part |
| **marchiq** | log segments vs blobs; no offset consumer |
| **CDC demo** | out of this repo; compose marchisql WAL → marchiq when needed |

Start at **slice 0** on `feat/skeleton`.
