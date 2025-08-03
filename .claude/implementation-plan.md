# Plano de Implementação - Rinha de Backend 2025

## Visão Geral
Este plano detalha a implementação incremental da solução, permitindo testes e validações a cada etapa.

## Fase 0: Limpeza e Preparação [30 min]

### Arquivos a Deletar
```bash
# Deletar toda estrutura atual exceto:
# - /payment-processor/
# - /rinha-test/
# - /docs/
# - /.claude/
# - .env
# - .gitignore

rm -rf cmd/ internal/ build/ configs/ migrations/ scripts/
rm -f Makefile Dockerfile docker-compose.yml go.mod go.sum
rm -f *.md (exceto os em /docs/)
```

### Estrutura Nova
```
rinha-backend-2025/
├── .claude/               # Planos e documentação
├── docs/                  # Documentação existente
├── payment-processor/     # Não tocar
├── rinha-test/           # Não tocar
└── .gitignore
```

## Fase 1: MVP Síncrono Simples [2 horas]

### Objetivo
Implementar versão mais simples possível que passe nos testes básicos.

### Estrutura
```
├── cmd/
│   └── api/
│       └── main.go
├── docker-compose.yml
├── Dockerfile
├── go.mod
└── Makefile
```

### Implementação
1. **main.go**: 
   - Fiber server básico
   - POST /payments → Chama default processor → Retorna 202
   - GET /payments-summary → Retorna contadores em memória

2. **docker-compose.yml**:
   - 1 instância API
   - Conectar na rede payment-processor

3. **Teste**:
   ```bash
   cd payment-processor && docker compose -f docker-compose-arm64.yml up -d
   cd .. && docker compose up --build
   cd rinha-test && ./run-tests.sh
   ```

### Critério de Sucesso
- Testes passam sem erros
- Consegue processar ao menos 1000 pagamentos

## Fase 2: Load Balancer + 2 Instâncias [1 hora]

### Adições
```
├── nginx/
│   └── nginx.conf
├── cmd/
│   └── api/
│       └── main.go (atualizado)
└── docker-compose.yml (atualizado)
```

### Implementação
1. **nginx.conf**: Round-robin entre 2 APIs
2. **docker-compose.yml**: nginx + 2 APIs
3. **Compartilhar estado**: Redis para contadores

### Teste
- Verificar distribuição de carga
- Confirmar que summary é consistente

## Fase 3: Redis + Processamento Assíncrono [3 horas]

### Adições
```
├── internal/
│   ├── queue/
│   │   └── redis.go
│   ├── models/
│   │   └── payment.go
│   └── storage/
│       └── redis.go
```

### Implementação
1. **API**: 
   - Recebe payment → RPUSH no Redis → 202
   - Summary lê contadores do Redis

2. **Worker** (novo serviço):
   - BLPOP da fila
   - Processa com default processor
   - Atualiza contadores

3. **Redis**:
   - Fila: `queue:payments`
   - Contadores: `stats:default:*`, `stats:fallback:*`

### Teste
- Carga de 5k requests
- Verificar fila não acumula
- Memory usage < 300MB

## Fase 4: Roteamento Inteligente [4 horas]

### Adições
```
├── internal/
│   ├── processor/
│   │   ├── client.go
│   │   ├── router.go
│   │   └── circuit_breaker.go
│   └── health/
│       └── checker.go
```

### Implementação
1. **Health Checker**:
   - Goroutine única checando a cada 5s
   - Cache resultado no Redis

2. **Circuit Breaker**:
   - Estados: CLOSED, OPEN, HALF_OPEN
   - Threshold: 3 falhas em 10s
   - Recovery: 5s

3. **Router**:
   ```go
   if defaultHealthy && defaultCircuit.Allow() {
       return processWithDefault()
   } else if fallbackHealthy && fallbackCircuit.Allow() {
       return processWithFallback()
   } else {
       return retryWithBackoff()
   }
   ```

### Teste com Falhas
```bash
# Terminal 1: Simular falhas
curl -X PUT http://localhost:8001/admin/configurations/failure \
  -H "X-Rinha-Token: 123" \
  -d '{"failure": true}'

# Terminal 2: Rodar teste
cd rinha-test && ./run-tests.sh
```

## Fase 5: Otimização de Performance [3 horas]

### Focos
1. **Object Pooling**:
   ```go
   var paymentPool = sync.Pool{
       New: func() interface{} {
           return &Payment{}
       },
   }
   ```

2. **Batch Redis Updates**:
   ```go
   pipe := redis.Pipeline()
   pipe.Incr("stats:default:count")
   pipe.IncrByFloat("stats:default:amount", amount)
   pipe.Exec()
   ```

3. **JSON Otimizado**:
   - Trocar encoding/json por easyjson
   - Gerar marshalers

4. **Tuning**:
   - GOGC=200 (menos GC)
   - GOMAXPROCS=2
   - Worker pool size: 100

### Benchmarks
```bash
# Medir p99
go test -bench=. -benchtime=10s ./...

# Profiling
go tool pprof -http=:8080 cpu.prof
```

## Fase 6: PostgreSQL Backup [2 horas]

### Adições
```
├── internal/
│   └── database/
│       ├── postgres.go
│       └── migrations/
│           └── 001_payments.sql
```

### Implementação
1. **Write-behind**:
   - Worker separa batch a cada 100 payments
   - Insert batch no PostgreSQL
   - Não bloquear processamento principal

2. **Recovery**:
   - Na inicialização, ler PG se Redis vazio
   - Reconstruir contadores

### Schema Mínimo
```sql
CREATE TABLE payments (
    correlation_id UUID PRIMARY KEY,
    amount DECIMAL(10,2),
    processor VARCHAR(10),
    processed_at TIMESTAMPTZ
);
```

## Fase 7: Ajuste Fino [2 horas]

### Tarefas
1. **Profiling Final**:
   - CPU profile
   - Memory profile
   - Trace requests

2. **Ajustar Recursos**:
   ```yaml
   # Distribuição final otimizada
   nginx: 0.10 CPU, 10MB
   api-1: 0.25 CPU, 50MB
   api-2: 0.25 CPU, 50MB
   redis: 0.30 CPU, 50MB
   postgres: 0.30 CPU, 50MB
   worker: 0.30 CPU, 40MB
   ```

3. **Timeout Tuning**:
   - Medir latências reais
   - Ajustar timeouts por processor

4. **Buffer Sizes**:
   - Redis connection pool
   - Channel buffers
   - HTTP client pool

## Fase 8: Testes de Stress [1 hora]

### Cenários
1. **Carga Máxima**:
   ```bash
   # 50k requests em 60s
   k6 run -u 1000 -d 60s test.js
   ```

2. **Chaos Testing**:
   - Derrubar Redis por 5s
   - Alternar falhas nos processors
   - Memory pressure

3. **Validação Final**:
   - Consistência após 100k payments
   - Zero logs de erro
   - p99 < target

## Marcos de Validação

### Após cada fase:
- [ ] Testes passando
- [ ] Memory < limite
- [ ] CPU < 80%
- [ ] Logs sem erros
- [ ] docker-compose up funciona

### Métricas Target:
- p99: < 5ms (12% bônus)
- Default usage: > 90%
- Success rate: > 99.9%
- Throughput: > 10k/s

## Rollback Plan

Se uma fase falhar:
1. Git stash mudanças
2. Voltar para última versão funcional
3. Identificar problema específico
4. Tentar approach alternativo

## Tempo Total Estimado: ~18 horas

Dividido em 2-3 dias de trabalho focado.