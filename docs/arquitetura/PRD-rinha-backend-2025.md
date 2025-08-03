# Product Requirements Document (PRD) - Rinha de Backend 2025

## 1. Visão Geral do Projeto

### 1.1 Objetivo
Desenvolver um sistema de intermediação de pagamentos que minimize custos de transação e maximize a performance, escolhendo inteligentemente entre dois processadores de pagamento com diferentes taxas e níveis de disponibilidade.

### 1.2 Contexto do Desafio
- **Pontuação Principal**: Maximizar lucro (mais pagamentos processados com menor taxa)
- **Penalizações**: 35% de multa sobre lucro total por inconsistências
- **Bônus Performance**: 2% por ms abaixo de 11ms no p99 (máx 20% em 1ms)

## 2. Requisitos Técnicos Críticos

### 2.1 Restrições de Recursos
- CPU Total: 1.5 unidades
- Memória Total: 350MB
- Mínimo 2 instâncias web
- Porta exposta: 9999
- Rede: bridge mode apenas

### 2.2 APIs a Implementar

#### POST /payments
```json
Request:
{
    "correlationId": "uuid-v4",
    "amount": 19.90
}
Response: HTTP 2XX (qualquer corpo)
```

#### GET /payments-summary
```json
Response:
{
    "default": {
        "totalRequests": 43236,
        "totalAmount": 415542345.98
    },
    "fallback": {
        "totalRequests": 423545,
        "totalAmount": 329347.34
    }
}
```

### 2.3 Integração com Payment Processors

#### Endpoints Disponíveis:
- **Default**: http://payment-processor-default:8080 (taxa menor)
- **Fallback**: http://payment-processor-fallback:8080 (taxa maior)

#### POST /payments (processor)
```json
Request:
{
    "correlationId": "uuid-v4",
    "amount": 19.90,
    "requestedAt": "2025-07-15T12:34:56.000Z"
}
```

#### GET /payments/service-health
- Rate limit: 1 chamada/5 segundos
- Retorna: `failing` (boolean) e `minResponseTime` (ms)

## 3. Estratégia de Vitória

### 3.1 Objetivos Primários
1. **Minimizar Taxa**: Priorizar processor default sempre que possível
2. **Maximizar Throughput**: Processar o máximo de pagamentos no tempo disponível
3. **Garantir Consistência**: Zero inconsistências para evitar multa de 35%
4. **Otimizar Performance**: Almejar p99 < 11ms para bônus

### 3.2 Decisões Arquiteturais Chave

#### 3.2.1 Processamento Assíncrono
- **Decisão**: Implementar fila interna com workers pool
- **Justificativa**: Permite receber pagamentos rapidamente (baixo p99) e processar em background

#### 3.2.2 Storage de Alta Performance
- **Decisão**: Redis como storage principal + PostgreSQL para durabilidade
- **Justificativa**: Redis oferece latência sub-ms, PostgreSQL garante consistência

#### 3.2.3 Estratégia de Roteamento Inteligente
- **Circuit Breaker Adaptativo**: 
  - Monitorar taxa de erro por janela deslizante
  - Abrir circuito com 3 falhas em 10 segundos
  - Half-open após 5 segundos
- **Health Check Otimizado**:
  - Check único a cada 5 segundos compartilhado entre workers
  - Cache do estado por 4.9 segundos
- **Timeout Dinâmico**:
  - Começar com timeout baseado no minResponseTime
  - Ajustar baseado no p95 observado

#### 3.2.4 Distribuição de Recursos
```yaml
nginx: 0.15 CPU, 15MB
api-1: 0.30 CPU, 60MB  
api-2: 0.30 CPU, 60MB
redis: 0.25 CPU, 40MB
postgres: 0.25 CPU, 40MB
worker: 0.25 CPU, 35MB
Total: 1.50 CPU, 300MB (50MB buffer)
```

## 4. Arquitetura Proposta

### 4.1 Componentes

#### 4.1.1 API Gateway (nginx)
- Load balancing round-robin
- Health checks nas APIs
- Timeout agressivo de 10ms

#### 4.1.2 API Servers (2x)
- Framework: Fiber v2 (mais rápido que Echo)
- Responsabilidades:
  - Validar entrada
  - Enqueue no Redis
  - Retornar 202 Accepted imediatamente
  - Servir /payments-summary do Redis

#### 4.1.3 Worker Service
- Pool de goroutines (100 workers)
- Processar fila Redis (BLPOP)
- Implementar lógica de roteamento
- Atualizar contadores no Redis

#### 4.1.4 Redis
- Estruturas:
  - `queue:payments` - Lista para fila
  - `state:default:requests` - Contador
  - `state:default:amount` - Soma
  - `state:fallback:requests` - Contador
  - `state:fallback:amount` - Soma
  - `health:default` - Cache health
  - `health:fallback` - Cache health
  - `circuit:default` - Estado circuit breaker
  - `circuit:fallback` - Estado circuit breaker

#### 4.1.5 PostgreSQL
- Tabela simples para auditoria
- Write-behind assíncrono
- Usado apenas para recovery em caso de crash

### 4.2 Fluxo de Processamento

1. **Recepção (API)**:
   ```
   Request → Validate → RPUSH Redis → Return 202
   Target: < 1ms
   ```

2. **Processamento (Worker)**:
   ```
   BLPOP → Check Circuit → Route → Process → Update Counters
   ```

3. **Roteamento Inteligente**:
   ```
   if default.circuit == CLOSED:
       try default with timeout
   else if fallback.circuit == CLOSED:
       try fallback with timeout
   else:
       wait and retry (max 3x)
   ```

## 5. Otimizações Específicas

### 5.1 Performance
- Pré-alocação de memória
- Object pooling para requests/responses
- Batch updates no Redis (pipeline)
- JSON encoding otimizado (easyjson)

### 5.2 Resiliência
- Retry com backoff exponencial
- Dead letter queue para falhas permanentes
- Graceful shutdown com drain da fila
- Health checks com degradação gradual

### 5.3 Observabilidade
- Métricas in-memory (sem Prometheus)
- Logs estruturados mínimos
- Tracing simplificado para debug

## 6. Plano de Testes

### 6.1 Testes de Carga Local
1. Simular 10k requests/segundo
2. Introduzir falhas nos processors
3. Validar consistência dos contadores
4. Medir p99 real

### 6.2 Cenários de Falha
1. Ambos processors down
2. Degradação gradual de performance
3. Falhas intermitentes
4. Rate limit do health check

## 7. Riscos e Mitigações

| Risco | Impacto | Mitigação |
|-------|---------|-----------|
| Memory leak | Alto | Profiling contínuo, limites rígidos |
| Redis SPOF | Alto | Persistência, recovery rápido |
| Network latency | Médio | Timeouts agressivos, retry local |
| Inconsistência | Alto | Transações Redis, validação dupla |

## 8. Métricas de Sucesso

1. **Taxa de Sucesso**: > 99.9%
2. **Uso Default**: > 90% quando disponível
3. **p99 Latência**: < 5ms (alvo 12% bônus)
4. **Inconsistências**: 0
5. **CPU Usage**: < 80% sustentado
6. **Memory**: < 300MB total

## 9. Decisões de Trade-off

1. **Durabilidade vs Performance**: 
   - Escolha: Performance (Redis primary, PG backup)
   - Justificativa: Teste curto, recovery possível

2. **Complexidade vs Simplicidade**:
   - Escolha: Simplicidade onde possível
   - Justificativa: Menos bugs, mais fácil otimizar

3. **Fairness vs Throughput**:
   - Escolha: Throughput
   - Justificativa: Pontuação baseada em volume

## 10. Tecnologias Escolhidas

- **Linguagem**: Go 1.23
- **Web Framework**: Fiber v2
- **Load Balancer**: nginx
- **Cache/Queue**: Redis 7
- **Database**: PostgreSQL 16
- **Serialization**: easyjson
- **HTTP Client**: fasthttp

Esta arquitetura foi desenhada especificamente para vencer a Rinha, priorizando performance e eficiência de custos sobre práticas tradicionais de produção.