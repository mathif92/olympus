# OlympusStore Database Implementation Plan

## Current Status: Phase 1 Complete ✅

### Completed Tasks (Phase 1):
- [x] Added dependencies to go.mod (lib/pq, redis/go-redis/v9)
- [x] Created `pkg/database_backend.go` with HybridStorageBackend
- [x] Created database package with Client struct

## Remaining Work

---

## Phase 2: Complete HybridBackend Implementation

### 2.1 Fix Imports and Complete Storage Backend

**File**: `pkg/database_backend.go`

**Actions Needed:**
1. Remove unused imports (`database/sql`, `redis.Client`)
2. Update the `Client` type to use the actual sql.DB type instead of custom wrapper
3. Implement proper PostgreSQL query execution
4. Complete all three StorageBackend interface methods: `StoreStream`, `Retrieve`, `Exists`

**Current Issues:**
- Line 21: `DB *Client` references non-exported type
- Line 90: `h.DB.DB.Exec()` assumes Client has nested DB field (needs fixing)
- Need to import `github.com/lib/pq` for PostgreSQL driver

---

## Phase 3: Update main.go to Wire Up Database

### 3.1 Modify cmd/app/main.go

**Current Code**:
```go
func main() {
    backend := pkg.NewLocalFSBackend(storageBaseDir)
    service := pkg.NewStorageService(backend)
    objHandler := handler.NewObjectHandler(service)
    mux.HandleFunc("/object", objHandler.HandleFunc)
    http.ListenAndServe(":8080", mux)
}
```

**Target Code**:
```go
func main() {
    // Initialize database client with config
    dbCfg := database.Config{
        PostgresURL: os.Getenv("POSTGRES_DSN"),  // or custom connection string
        RedisURL:    os.Getenv("REDIS_URL"),      // default: redis:6379
        PoolMax:     getEnvInt("POOL_MAX", 20),   // custom pool size
        PoolMin:     getEnvInt("POOL_MIN", 5),
    }
    
    dbClient, err := database.NewClient(dbCfg)
    if err != nil {
        log.Fatalf("Database initialization failed: %v", err)
    }
    defer dbClient.Close()

    // Initialize storage backend
    fsBackend := pkg.NewLocalFSBackend(storageBaseDir)
    
    // Create hybrid backend (stores data on FS, metadata in Postgres)
    hybridBackend := pkg.NewHybridStorageBackend(
        storageBaseDir, 
        dbClient,       // PostgreSQL client
        dbClient.Cache, // Redis cache from same client
    )

    // Wrap in StorageService
    service := pkg.NewStorageService(hybridBackend)

    objHandler := handler.NewObjectHandler(service)
    mux.HandleFunc("/object", objHandler.HandleFunc)
    
    log.Printf("🚀 OlympusStore running on :8080...")
    http.ListenAndServe(":8080", nil)
}
```

---

## Phase 4: Add Quota Enforcement Middleware (Optional Enhancement)

### 4.1 Create Account/Space Management in Database Layer

**New Function**: `GetAccountQuota(ctx, accountId string)` - Returns storage limit and used bytes
**New Function**: `CheckAndDeductQuota(spaceID, newSize int64)` - Validates quota before write

---

## Phase 5: Add Health Check Endpoint

### 5.1 Create /health HTTP Endpoint

```go
func handler.HealthHandler(w http.ResponseWriter, r *http.Request) {
    // Check PostgreSQL connectivity
    if err := h.DB.Ping(); err != nil {
        http.Error(w, "PostgreSQL unavailable", http.StatusServiceUnavailable)
        return
    }
    
    // Check Redis connectivity  
    if err := h.Cache.Ping().Err(); err != nil {
        http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
        "postgres": "ok",
        "redis":   "ok",
    })
}
```

**Route**: `mux.HandleFunc("/health", handler.HealthHandler)`

---

## Phase 6: Run go mod tidy (Final Step)

After all changes are complete:

```bash
cd /home/mathias/dev/go/olympus/storage
go mod tidy
go build -o olympus-store ./cmd/app
./olympus-store -storage-dir ./storage/data
```

---

## Summary: 5 Files to Modify/Create

| # | File | Action | Priority |
|---|------|--------|----------|
| 1 | `pkg/database_backend.go` | Clean up imports, export `Client` type, implement all methods | High |
| 2 | `cmd/app/main.go` | Add database initialization and hybrid backend wiring | High |
| 3 | `database/client.go` (new) | Export proper types for external use | Medium |
| 4 | `cmd/app/http_handler.go` (new or modify object.go) | Add /health endpoint | Low |
| 5 | `go.mod` | Run `go mod tidy` after changes | Final step |

---

## Next Steps: Choose One Path

**Option A - Continue Implementation Now:**
```bash
# 1. Update database_backend.go to fix Client type export
# 2. Update main.go to wire up database
# 3. Run go mod tidy
# 4. Test with docker-compose up
```

**Option B - Save for Later:**
- Review this file
- Come back when ready to implement
- The SQL migrations are already ready and will auto-run from docker-compose

**Option C - Simplified Approach:**
- Skip Redis caching initially (only PostgreSQL for metadata)
- Only implement `StoreStream` with PostgreSQL upserts
- Add `Retrieve` via filesystem + simple exists check
- Come back to caching later once basic ops work

---

## Environment Variables Required

Add these to your `.env` or docker-compose:

```bash
POSTGRES_DSN=host=localhost port=5432 user=olympus password=olympus_db password=olmpy_olym
REDIS_URL=redis://localhost:6379
POOL_MAX=20
POOL_MIN=5
```

---

## Testing Checklist

After implementation:

- [ ] `go build` succeeds with no errors
- [ ] `docker-compose up` starts PostgreSQL, Redis, and service
- [ ] `/health` endpoint returns 200 OK
- [ ] PUT /object/{bucket}/{key} works (uploads + metadata in Postgres)
- [ ] GET /object/{bucket}/{key} returns cached data on second request
- [ ] Object count queryable via PostgreSQL: `SELECT COUNT(*) FROM objects`

---

**Last Updated**: 2024-12-06
