# Storage-Worker Replication Internals

This document describes how Amphora's on-premise object storage layer
replicates and protects data. Storage is backed by a distributed MinIO cluster
whose server pods ("storage workers") each own a `PersistentVolumeClaim`. The
API gateway pods are stateless: they never hold object bytes, they only route
PUT/GET to the cluster's S3 API and mirror metadata into Postgres/Redis.

```
┌──────────────────────────────────────────────────────────────┐
│                 Olympus API Gateway pods                     │
│    (stateless: metadata -> Postgres, cache -> Redis)         │
└───────────────────────────┬──────────────────────────────────┘
                            │  S3 (minio-go)
┌───────────────────────────▼──────────────────────────────────┐
│            Distributed MinIO  (the storage workers)          │
│   StatefulSet { minio-0 .. minio-N }, each with a RWO PVC    │
│   erasure coding across drives/nodes                         │
└──────────────────────────────────────────────────────────────┘
```

---

## 1. Erasure coding (data-at-rest protection)

MinIO stores each object as a set of **shards** spread across drives/nodes using
**Reed-Solomon erasure coding**. With a shard/parity ratio `n : m`:

- Data is split into **n** data shards.
- **m** parity shards are computed.
- The object remains readable while **any n** of the `n+m` shards survive.

Common configurations for the bundled chart (`minio.distributedNodes`):

| Nodes | Parity | Tolerated data shard loss | Tolerated parity/drive loss |
|-------|--------|---------------------------|-----------------------------|
| 4     | 2      | 2                         | 2                           |
| 6     | 2      | 2                         | 4                           |
| 8     | 2      | 2                         | 6                           |
| 8     | 4      | 3                         | 5                           |

`distributedNodes=4` is the minimum for erasure coding and protects against up
to 2 drive/node failures.

### The write path (per object)

1. The gateway computes a SHA-256 ETag while streaming the body to a temp file,
   then calls `PutObject` on the MinIO service.
2. One worker acts as the coordinator for the request and computes the erasure
   layout for the object over the cluster's drive set.
3. The object is chunked; data shards are written to their assigned drives and
   parity shards to the remaining ones. Shards are staged and then committed
   atomically once the required set is durable.
4. The PUT is acknowledged only after the required number of shards is durable.

### Drive selection / placement

Placement uses a deterministic **consistent-hashing (rendezvous) distribution**
of shards across the erasure set. The layout is stable for the lifetime of the
drives. Adding a node extends the layout and triggers a background **rebalance**
that moves shards so the new node participates. Reads resolve shards from the
same layout, so no central metadata lookup is required for object data.

---

## 2. Write consistency

- A PUT is acknowledged only after a quorum of shards is durable.
- A concurrent GET during a PUT observes either the old or the new version,
  never a partially-erased mix (write ordering + per-shard commit bounds).
- Metadata (Postgres) and content (MinIO) are written independently; metadata is
  the source of truth for lookup/listing. Content is immutable per ETag, so a
  mismatch is detectable and re-syncable by a repair job.

---

## 3. Self-healing and bit-rot detection

- Each shard carries checksums; MinIO periodically scans drives and detects
  bit-rot. If a shard is corrupt or missing but the remaining shards are still
  at least `n`, the object is reconstructed and the missing shard rewritten.
- On drive loss, scanning the disk layout identifies affected objects and
  re-creates their shards on healthy drives.

---

## 4. Replication modes

### 4.1 Intra-cluster replication

Handled entirely by erasure coding above. This protects against drive/node loss
but **not** full site or region loss.

### 4.2 Site / DR replication (cross-cluster)

For availability-zone or region-scale durability, enable MinIO **server-side
bucket replication**:

- Configure one-way (or two-way) replication rules on the bucket.
- Every uploaded object triggers an asynchronous replication event to the remote
  site.
- Reads stay local; a replica site can be promoted to serve if the primary is
  lost.

This is a separate concern from erasure coding and is recommended for true
on-premise DR rather than relying on a single cluster.

---

## 5. Versioning and object lock

- **Versioning**: when enabled, overwrites create a new version instead of
  replacing; each version is immutable, matching the `version_id` semantics the
  API layer models (`LATEST` pointer).
- **Object lock / WORM**: retention-based compliance. Enable only if SLAs
  require it.

---

## 6. How the API gateway stays consistent

The gateway does **not** hold object bytes, so it can scale to zero and back
without data divergence. Its responsibilities:

1. **PUT**: compute ETag + size, upload via S3, then upsert metadata into
   Postgres (keyed by `(space_id, key_path)`). If the upload fails, no metadata
   row is written, so no orphan metadata exists.
2. **GET**: check existence (S3 stat or Postgres), then stream from S3.
3. **Repair/orphan reconciliation** (future work): a scheduled job compares
   Postgres `objects` against S3 listings to heal deleted/orphaned objects and
   detect ETag drift between the DB and stored content.

---

## 7. Scaling and rebalancing

- **Scale-out**: add MinIO worker replicas (`minio.distributedNodes` +
  `storageSize`). Each pod binds its own RWO PVC; spread pods across
  nodes/zones via `topologySpreadConstraints`/node affinity.
- **Rebalance**: when a node is added, MinIO redistributes shards in the
  background. Plan for capacity headroom during rebalancing.
- The **gateway** scales freely (HPA on CPU) because it is stateless.

---

## 8. Failure scenarios summary

| Failure                     | Detected by                      | Recovered by                          | Read availability            |
|-----------------------------|----------------------------------|---------------------------------------|------------------------------|
| A single drive fails        | erasure scan / checksum mismatch | reconstruct from surviving shards     | yes (erasure coding)         |
| Up to parity nodes down     | layout / liveness                | heal onto remaining drives            | yes (≤ parity tolerated)     |
| More than parity lost       | layout shows missing shards      | site replication or backup            | no                           |
| Whole site down             | health / replication lag         | promote DR site replica               | via site replica             |

---

## 9. Trade-offs and decisions for Amphora

- **Blob layer owns sharding**: MinIO keeps shard placement and healing logic,
  so Postgres only stores rich, indexable, multi-tenant metadata (accounts,
  spaces, audit, quota).
- **Metadata out-of-band**: relational queries (counts, ACLs, listing, audit)
  stay in Postgres, which is easier to index, transaction, and secure per
  tenant.
- **Consistency model**: strong metadata + eventually-consistent content
  replication; the two converge because content is immutable per ETag.
- **Gateway statelessness** is what makes horizontal scale-out and rolling
  deploys safe on-prem.

---

### Related files

- `pkg/minio_backend.go` — the S3 gateway backend.
- `deploy/helm/olympus/` — MinIO workers as a StatefulSet with per-pod PVCs.
- `DB_COMPARISON.md` — why PostgreSQL + Redis + MinIO.