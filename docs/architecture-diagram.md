# Arquitetura do Sistema - Rinha Backend 2025

## Visão Geral da Arquitetura

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLIENTE HTTP                                    │
└─────────────────────┬───────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           NGINX LOAD BALANCER                              │
│                         (2 instâncias da API)                              │
└─────────────────────┬───────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                        ECHO HTTP SERVER                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        ROUTES                                       │   │
│  │  • GET  /                    (Hello World)                          │   │
│  │  • GET  /health              (Health Check)                         │   │
│  │  • POST /payments            (Create Payment)                       │   │
│  │  • GET  /payments-summary    (Payment Summary)                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────┬───────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          SERVER LAYER                                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐ │
│  │   Database      │  │  Worker Pool    │  │    Processor Service       │ │
│  │   Service       │  │  (5 workers)    │  │   (Default + Fallback)     │ │
│  │                 │  │                 │  │                             │ │
│  │ • CreatePayment │  │ • Job Queue     │  │ • Health Checks             │ │
│  │ • UpdateStatus  │  │   (1000 cap)    │  │ • Retry Logic               │ │
│  │ • CompletePaymt │  │ • Async Process │  │ • Circuit Breaker           │ │
│  │ • GetSummary    │  │ • Graceful Stop │  │                             │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────────────────┘ │
└─────────┬───────────────────────┬─────────────────────────┬─────────────────┘
          │                       │                         │
          ▼                       │                         ▼
┌─────────────────┐              │              ┌─────────────────────────────┐
│   PostgreSQL    │              │              │     PAYMENT PROCESSORS      │
│                 │              │              │                             │
│ ┌─────────────┐ │              │              │  ┌─────────────────────┐   │
│ │   payments  │ │              │              │  │      DEFAULT        │   │
│ │   table     │ │              │              │  │ (Lower fees)        │   │
│ │             │ │              │              │  │ :8080/payments      │   │
│ │ • id        │ │              │              │  │ :8080/.../health    │   │
│ │ • corr_id   │ │              │              │  └─────────────────────┘   │
│ │ • amount    │ │              │              │                             │
│ │ • fee       │ │              │              │  ┌─────────────────────┐   │
│ │ • proc_type │ │              │              │  │     FALLBACK        │   │
│ │ • status    │ │              │              │  │ (Higher fees)       │   │
│ │ • timestamps│ │              │              │  │ :8080/payments      │   │
│ └─────────────┘ │              │              │  │ :8080/.../health    │   │
└─────────────────┘              │              │  └─────────────────────┘   │
                                 │              └─────────────────────────────┘
                                 ▼
                    ┌─────────────────────────────┐
                    │      ASYNC PROCESSING       │
                    │                             │
                    │  1. Payment → "pending"     │
                    │  2. Worker picks up job     │
                    │  3. Status → "processing"   │
                    │  4. Try Default processor   │
                    │  5. If fail → Try Fallback  │
                    │  6. Success → "completed"   │
                    │  7. Fail → "failed"         │
                    └─────────────────────────────┘
```

## Fluxo de Dados Detalhado

### 1. **Recebimento de Pagamento**
```
POST /payments
├── Validação do request
├── Criação do Payment (status: "pending")
├── Salvamento no PostgreSQL
├── Envio para Worker Pool
└── Retorno HTTP 202 (Accepted)
```

### 2. **Processamento Assíncrono**
```
Worker Pool
├── Worker pega job da fila
├── Atualiza status para "processing"
├── Processor Service processa:
│   ├── Health check dos processadores
│   ├── Tentativa com Default processor
│   │   ├── Retry (3x com backoff)
│   │   └── Se falhar → Fallback
│   └── Tentativa com Fallback processor
│       ├── Retry (3x com backoff)
│       └── Se falhar → status "failed"
└── Sucesso → Atualiza com fee e processor_type
```

### 3. **Consulta de Resumo**
```
GET /payments-summary?startDate=2025-01-01&endDate=2025-01-31
├── Parse de parâmetros de data
├── Query agregada no PostgreSQL
└── Retorno agrupado por processor_type
```

## Componentes e Responsabilidades

### **cmd/api/main.go**
- Entry point da aplicação
- Graceful shutdown
- Inicialização do servidor

### **internal/server/**
- `server.go`: Configuração do servidor HTTP e worker pool
- `routes.go`: Definição de rotas e handlers

### **internal/models/**
- `payment.go`: Structs e types para pagamentos

### **internal/database/**
- `database.go`: Service layer para PostgreSQL
- Interface e implementação para operações de BD

### **internal/processors/**
- `client.go`: Cliente HTTP para comunicação com processadores
- `service.go`: Lógica de retry, fallback e health checks

### **internal/workers/**
- `payment_worker.go`: Worker pool para processamento assíncrono

## Padrões Arquiteturais Utilizados

1. **Layered Architecture**: Separação clara entre apresentação, negócio e dados
2. **Worker Pool Pattern**: Processamento assíncrono com pool de workers
3. **Circuit Breaker**: Health checks e failover entre processadores
4. **Repository Pattern**: Abstração do acesso a dados
5. **Graceful Shutdown**: Finalização ordenada dos workers

## Configurações de Performance

- **Worker Pool**: 5 workers simultâneos
- **Queue Size**: 1000 jobs
- **Connection Pool**: PostgreSQL com pooling automático
- **Timeouts**: 10s para HTTP requests, 30s para processamento
- **Health Check Cache**: 5s de cooldown

## Recursos de Monitoramento

- Health endpoint (`/health`) com estatísticas do BD
- Logs estruturados em cada worker
- Métricas de performance dos processadores