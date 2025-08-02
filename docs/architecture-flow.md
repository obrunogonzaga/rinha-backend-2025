# Rinha Backend 2025 - Architecture Flow

## System Overview

```mermaid
graph TB
    subgraph LB ["Load Balancer"]
        Nginx["Nginx<br/>0.30 CPU / 50MB<br/>:9999→80"]
    end

    subgraph API ["API Layer"]
        API1["API-1<br/>0.45 CPU / 90MB<br/>:8081→8080"]
        API2["API-2<br/>0.45 CPU / 90MB<br/>:8082→8080"]
    end

    subgraph Storage ["Storage"]
        PG[("PostgreSQL<br/>0.15 CPU / 80MB<br/>:5432")]
        Redis[("Redis<br/>0.15 CPU / 40MB<br/>:6379")]
    end

    subgraph Workers ["Workers"]
        Worker["5 Workers<br/>Redis Consumer"]
        CB1["Circuit Breaker<br/>Default"]
        CB2["Circuit Breaker<br/>Fallback"]
        HM["Health Monitor<br/>5s interval"]
    end

    subgraph External ["External"]
        PP1["Default Processor<br/>3% fee<br/>:8080"]
        PP2["Fallback Processor<br/>5% fee<br/>:8080"]
    end

    %% Flow connections
    Nginx --> API1
    Nginx --> API2
    API1 --> PG
    API2 --> PG
    API1 --> Redis
    API2 --> Redis
    Worker --> Redis
    Worker --> PG
    Worker --> CB1
    Worker --> CB2
    CB1 --> PP1
    CB2 --> PP2
    HM --> PP1
    HM --> PP2
    HM --> Redis

    %% Styling
    classDef api fill:#4CAF50,stroke:#333,stroke-width:2px,color:#fff
    classDef storage fill:#2196F3,stroke:#333,stroke-width:2px,color:#fff
    classDef worker fill:#FF9800,stroke:#333,stroke-width:2px,color:#fff
    classDef external fill:#f44336,stroke:#333,stroke-width:2px,color:#fff
    classDef lb fill:#9C27B0,stroke:#333,stroke-width:2px,color:#fff
    
    class API1,API2 api
    class PG,Redis storage
    class Worker,CB1,CB2,HM worker
    class PP1,PP2 external
    class Nginx lb
```

## Payment Processing Flow (Swimlane)

```mermaid
flowchart TD
    subgraph Client ["🌐 Client"]
        C1[POST /payments]
        C2[GET /payments-summary]
    end
    
    subgraph Nginx ["⚖️ Load Balancer"]
        N1[Round Robin]
    end
    
    subgraph API ["🚀 API Layer"]
        A1[Validate Request]
        A2[Create Payment]
        A3[Publish Job]
        A4[Return 202]
        A5[Query Summary]
    end
    
    subgraph Database ["🗄️ PostgreSQL"]
        D1[Insert Payment]
        D2[Update Status]
        D3[Aggregate Data]
    end
    
    subgraph Queue ["📦 Redis"]
        Q1[Queue Job]
        Q2[Cache Health]
        Q3[Consume Job]
    end
    
    subgraph Workers ["⚙️ Worker Pool"]
        W1[Process Job]
        W2[Check Health]
        W3[Route Request]
    end
    
    subgraph CircuitBreaker ["🔌 Circuit Breakers"]
        CB1[Default Circuit]
        CB2[Fallback Circuit]
    end
    
    subgraph External ["🔗 Payment Processors"]
        E1[Default Processor]
        E2[Fallback Processor]
    end
    
    subgraph Monitor ["📊 Health Monitor"]
        M1[Check Processors]
        M2[Update Cache]
    end

    %% Payment Flow
    C1 --> N1
    N1 --> A1
    A1 --> A2
    A2 --> D1
    A2 --> A3
    A3 --> Q1
    A3 --> A4
    
    Q3 --> W1
    W1 --> W2
    W2 --> Q2
    W2 --> W3
    W3 --> CB1
    W3 --> CB2
    CB1 --> E1
    CB2 --> E2
    E1 --> W1
    E2 --> W1
    W1 --> D2
    
    %% Summary Flow
    C2 --> N1
    N1 --> A5
    A5 --> D3
    
    %% Health Monitoring
    M1 --> E1
    M1 --> E2
    M1 --> M2
    M2 --> Q2

    %% Styling
    classDef client fill:#E3F2FD,stroke:#1976D2,stroke-width:2px
    classDef nginx fill:#F3E5F5,stroke:#7B1FA2,stroke-width:2px
    classDef api fill:#E8F5E8,stroke:#388E3C,stroke-width:2px
    classDef database fill:#E1F5FE,stroke:#0277BD,stroke-width:2px
    classDef queue fill:#FFF3E0,stroke:#F57C00,stroke-width:2px
    classDef worker fill:#FFF8E1,stroke:#FBC02D,stroke-width:2px
    classDef circuit fill:#EFEBE9,stroke:#5D4037,stroke-width:2px
    classDef external fill:#FFEBEE,stroke:#D32F2F,stroke-width:2px
    classDef monitor fill:#F1F8E9,stroke:#689F38,stroke-width:2px
    
    class C1,C2 client
    class N1 nginx
    class A1,A2,A3,A4,A5 api
    class D1,D2,D3 database
    class Q1,Q2,Q3 queue
    class W1,W2,W3 worker
    class CB1,CB2 circuit
    class E1,E2 external
    class M1,M2 monitor
```

## Resource Allocation

| Component | CPU | Memory | Port | Instances |
|-----------|-----|--------|------|-----------|
| Nginx | 0.30 | 50MB | 9999 | 1 |
| API-1 | 0.45 | 90MB | 8081 | 1 |
| API-2 | 0.45 | 90MB | 8082 | 1 |
| PostgreSQL | 0.15 | 80MB | 5432 | 1 |
| Redis | 0.15 | 40MB | 6379 | 1 |
| **Total** | **1.50** | **350MB** | - | **5** |

## Data Flow

### Payment Processing
1. **Client** → POST `/payments` → **Nginx**
2. **Nginx** → Round robin → **API Instance**
3. **API** → Create payment → **PostgreSQL**
4. **API** → Publish job → **Redis Queue**
5. **Worker** → Consume job → **Redis**
6. **Circuit Breaker** → Route to processor → **External API**
7. **Worker** → Update status → **PostgreSQL**

### Health Monitoring
1. **Health Monitor** → Check every 5s → **External APIs**
2. **Health Monitor** → Cache status → **Redis** (TTL 30s)
3. **APIs** → Read health cache → **Redis**
4. **Circuit Breakers** → Use health status → Decision logic

## Key Components

### Circuit Breakers
- **Default**: 3 max req, 30s timeout, 60% failure threshold
- **Fallback**: 5 max req, 45s timeout, 80% failure threshold

### Redis Usage
- **Queue**: `payments:queue` - Job persistence
- **Health Cache**: `health:default`, `health:fallback` - Status cache

### Worker Pool
- **5 Workers** consuming from Redis
- **Block-pop** with 10s timeout
- **Async processing** with status updates

## External Dependencies

- **payment-processor-default**: Primary processor (3% fee)
- **payment-processor-fallback**: Backup processor (5% fee)
- **Docker Network**: `payment-processor` (external)

## Endpoints

- `GET /health` - Database health
- `POST /payments` - Create payment (202 Accepted)
- `GET /payments-summary` - Aggregated data
- `DELETE /payments` - Clear all payments

## Features

✅ **Job Persistence** - Redis queues survive restarts  
✅ **Circuit Protection** - Intelligent failure handling  
✅ **Health Caching** - Efficient processor monitoring  
✅ **Load Balancing** - 2 API instances with Nginx  
✅ **Resource Compliance** - 1.5 CPU / 350MB total