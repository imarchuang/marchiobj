# Object store internals

marchiobj is a teaching model of an S3-shaped **blob store**: opaque bytes plus a
small metadata index. It is not IAM, not erasure coding, and not AWS XML.

## Control plane vs data plane

| Plane | What | Where |
|---|---|---|
| Control | bucket records, object keys, etag/size/mtime, multipart session index | `bucket.json`, `meta/*.json`, `upload-index/` |
| Data | immutable blob bytes and in-flight parts | `blobs/{sha256}`, `multipart/{uploadId}/part-*` |

**LIST never opens blob files.** It scans `meta/` JSON. The key is a user path,
not a time partition (unlike marchilogs).

## Immutable PUT

1. Stream the body to `blobs/.put-*` while hashing **sha256**.
2. `fsync` and `rename` to `blobs/{hex}` (content-addressed; identical bytes are a no-op).
3. Atomically write `meta/{escaped-key}.json` with `{key, blobId, size, etag, mtime}`.
4. If the previous `blobId` is unreferenced, unlink it.

Readers never see a half-written blob: the metadata pointer moves only after the
rename. Replace is “new blob, then swap the pointer.”

## ETag and conditionals

`ETag` is the sha256 hex of the object bytes (quoted on the wire).

- `If-None-Match` on GET/HEAD → **304** when the current etag matches (`*` matches any).
- `If-Match` on GET/HEAD/PUT/DELETE → **412** when it does not match.
- `If-None-Match: *` on PUT → **412** if the key already exists.

## Range GET

`Range: bytes=start-end` seeks the blob file and returns **206** + `Content-Range`.
Unsatisfiable ranges return **416**. The process does not buffer the whole object.

## Multipart atomic complete

Parts live under `buckets/{b}/multipart/{uploadId}/` until complete:

1. Concatenate `part-00001…` into `blobs/.complete-*` (hash as we copy).
2. `fsync` + rename to `blobs/{sha256}`.
3. **Then** publish object metadata and delete the upload directory.

A crash during concat leaves at most a temp file. **The key is not listed** until
step 3. Abort is `RemoveAll` on that upload directory.

## On-disk layout

```text
{dataDir}/
  buckets/{bucket}/
    bucket.json
    meta/{url-escaped-key}.json
    blobs/{sha256}
    multipart/{uploadId}/meta.json
    multipart/{uploadId}/part-NNNNN
  upload-index/{uploadId}.json
```
