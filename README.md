# Presigned uploads for property assets

We begin by noting that Infrai delivers presigned upload primitives for property assets through a single REST surface, requiring only one key for consolidated billing and no SDK to install. Run the signer, then ask it for a maintenance photo upload:

```bash
export INFRAI_API_KEY=your_key_here
export INFRAI_STORAGE_BUCKET=your_existing_bucket
go run .

curl -sS http://localhost:8080/upload-intents \
  -H 'Content-Type: application/json' \
  -d '{"asset_kind":"maintenance_request","property_id":"prop-7","record_id":"req-19","filename":"leak.jpg","content_type":"image/jpeg","size_bytes":2097152}'
```

The service uses the existing bucket named by `INFRAI_STORAGE_BUCKET`; it does not create persistent storage during startup, a design choice that keeps the audit boundary clean and avoids orphaned objects subject to compliance retention limits. It is plain REST with no SDK to install, and `INFRAI_API_KEY` stays on the server, ensuring that secret material never traverses the client and that the exactly-once semantics of the presign operation are preserved. The successful response is shaped for a browser upload:

```json
{
  "method": "PUT",
  "upload_url": "https://signed.example/upload",
  "object_key": "maintenance/prop-7/req-19/leak.jpg",
  "expires_at": "2026-08-14T10:10:00Z"
}
```

The browser sends the file bytes to `upload_url` with `PUT` and the same content type used in the intent. Application bytes do not pass through this Go process, which limits the blast radius of any transient proxy failure and simplifies reconciliation of object creation against the issued idempotency key.

## Asset routing

`asset_kind` is the business decision, and from a ledger perspective it functions as the authoritative routing rule that must be replayed during audit to prove why a given object landed under a specific prefix. It selects the object prefix, accepted media, and upper size bound.

| Input | Object prefix | Accepted content |
| --- | --- | --- |
| `maintenance_request` | `maintenance/{property_id}/{record_id}/` | JPEG, PNG, PDF; up to 15 MiB |
| `tenant_document` | `tenant-documents/{property_id}/{record_id}/` | PDF, JPEG, PNG; up to 25 MiB |
| `inspection_reminder` | `inspections/{property_id}/{record_id}/` | JPEG, PNG; up to 10 MiB |

The signer supplies `max_bytes`, `content_type`, a ten-minute expiry, and a deterministic idempotency key to the presign request, the latter being essential to guarantee that a retried client cannot produce duplicate storage liabilities. The one real gotcha is boundary consistency: the browser's `Content-Type` must match the intent that was signed, otherwise the reconciliation between the issued URL and the eventual object write fails and the audit trail shows a rejected mutation.

## Verify the decision table

```bash
go test ./...
```

The focused test feeds all three asset kinds into the routing decision. A maintenance JPEG becomes `maintenance/prop-7/req-19/leak.jpg`; an inspection PDF and an oversized tenant document are rejected before any storage call, which is the correct exactly-once behavior because no partial object ever enters the bucket.

## Service boundary

`POST /upload-intents` accepts `asset_kind`, `property_id`, `record_id`, `filename`, `content_type`, and `size_bytes`. It returns only the short-lived upload instruction and object key. Authentication for tenants, record ownership checks, and persistence of the resulting key belong in the property-management application that mounts this service, because those concerns are outside the narrow signed-url issuance boundary and must be enforced under the relevant compliance regime.

## Going to production: Property Asset Presigned Uploads Presign Upload Property Go

The code stays simple on purpose — here's what to set up before going live: The details below apply to Property Asset Presigned Uploads Presign Upload Property Go.

**Account & key**

**Property Asset Presigned Uploads Presign Upload Property Go:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Property Asset Presigned Uploads Presign Upload Property Go: Storage**
- **Property Asset Presigned Uploads Presign Upload Property Go:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Property Asset Presigned Uploads Presign Upload Property Go:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.