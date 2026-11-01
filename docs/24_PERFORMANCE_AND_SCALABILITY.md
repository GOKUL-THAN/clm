# CLM-SPEC-24: PERFORMANCE & SCALABILITY SPECIFICATION
## Sub-250ms API Latency Targets, Horizontal Scaling, and High-Throughput Indexing

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-PERF-024  
**Classification:** Enterprise Performance Engineering Standard  

---

### 1. Performance Service Level Objectives (SLOs)

The platform is engineered to meet strict enterprise latency and throughput targets under high load:

| Operation Category | Target p95 Latency | Target p99 Latency | Max Sustained Throughput |
| :--- | :---: | :---: | :---: |
| **Contract Search & Filters** | < 80 ms | < 150 ms | 2,500 queries / sec |
| **Metadata CRUD API** | < 120 ms | < 250 ms | 1,500 req / sec |
| **Intake Request Conversion** | < 200 ms | < 400 ms | 500 conversions / sec |
| **Three-Way Match Verification** | < 150 ms | < 300 ms | 800 reconciliations / sec |
| **AI Contract Q&A Inference** | < 1,200 ms | < 2,500 ms | 100 inferences / sec |
| **In-VPC HSM Digital Signature**| < 350 ms | < 750 ms | 250 signatures / sec |

---

### 2. Horizontal Scaling Architecture

* **Stateless Microservices:** Go control plane services maintain zero local session state. Replicas scale dynamically from 3 to 50 pods based on CPU utilization (> 70%) and request concurrency.
* **Database Connection Pooling:** PgBouncer connection pooling layer handles up to 10,000 active client connections, reusing a pool of 200 backend PostgreSQL connections.
* **Read-Write Separation:** Write operations target the primary PostgreSQL database, while search queries and executive reporting read from read replicas.

---

### 3. High-Throughput Indexing & Caching

1. **In-Memory Cache:** Pre-approved clauses from `clause_library` and active DOA threshold rules are cached in memory with a 5-minute TTL, eliminating repetitive database queries.
2. **Partial Indexing:** High-frequency status queues (e.g. `WHERE status = 'pending'`) use partial B-tree indexes, maintaining small index footprints (< 5% of table size).

