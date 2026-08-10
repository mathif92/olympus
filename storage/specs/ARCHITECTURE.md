## 💾 Implementation Plan: OlympusStore Object Storage Service (S3-Compatible)

**Goal:** Implement an on-premise, S3-compatible object storage service in Go (`OlympusStore`). The service must prioritize robust streaming I/O for efficient handling of large byte payloads and maintain data integrity using standard Go patterns.

### 🚀 Architecture Overview
The system will follow a classic tiered architecture to ensure separation of concerns:

1.  **API Layer (`main.go`)**: Handles HTTP request routing, deserialization (metadata), authentication checks, and initial validation. It delegates all work to the service layer.
2.  **Service Layer (New package `service/`):** Contains the core business logic. This is where object key generation, content hashing (ETag/SHA256), chunking coordination, and error wrapping will happen. *This layer should depend only on interfaces.*
3.  **Storage Backend (New package `storage/`):** Responsible for the physical interaction with the underlying persistent storage. It implements the streaming write/read operations using Go's `io.Reader` and `io.Writer`.

### ⚙️ Phase Breakdown & Tasks

#### Task 1: Setup Core Structure and Key Management
*   **Objective:** Define the basic API handlers and establish reliable object key generation.
*   **Tasks:**
    1.  Define the core service struct/interface in a new location (e.g., `pkg/storage_service.go`).
    2.  Implement an object key generator function that takes the desired object name and returns a unique, hashed key (UUIDv4 or SHA-256 of the path).
*   **Files to Create/Modify:**
    *   `storage/cmd/app/main.go`: Initialize HTTP server and define routes (`GET /object/{key}`, `PUT /object`).
    *   `pkg/storage_service.go`: Contains the `StorageService` struct and key generation logic.

#### Task 2: Implement Object Upload (The PUT Path - Streamed)
*   **Objective:** Accept an incoming stream of bytes from an HTTP request body, calculate its hash, write it to disk efficiently via streaming, and save associated metadata.
*   **Tasks:**
    1.  Create a `PutObject` handler function in the service layer.
    2.  Utilize Go's streaming capabilities (`io.Copy` pattern) to read from the request body stream directly into the backend writer, avoiding reading all content into memory (critical for large files).
    3.  Integrate hashing: The write process must pass the data through an intermediate SHA-256 hash writer *before* it hits the actual file write mechanism.
    4.  Write the object in chunks to disk using `os.OpenFile` with a unique key.
    5.  Store necessary metadata (e.g., original user name, content type, ETag/hash) alongside the physical file location reference.

#### Task 3: Implement Object Download (The GET Path - Streamed)
*   **Objective:** Retrieve an object's stored stream from disk and serve it back to the client with correct HTTP headers.
*   **Tasks:**
    1.  Create a `GetObject` handler function that reads the key from the URL path.
    2.  Look up the unique key in the metadata index (requires changing the backend contract).
    3.  Open the file corresponding to the key using `os.Open`.
    4.  Serve the content by wrapping the file handle as an `http.ServeContent` writer, ensuring appropriate headers (`Content-Type`, `Content-Length`, etc.).

#### Task 4: Testing and Cleanup
*   **Objective:** Add unit tests for the service layer components (key generation, hashing) and integrate a basic health check endpoint.
*   **Tasks:** Write simple integration test that simulates an upload/download cycle using the new streaming write patterns.