# donation-service

Processamento de doações da SolidaryTech — **Hot Path / Caminho Crítico**
do desafio. É o único serviço com SLO formal definido (ver dashboard SRE em
`fiap-tc-5-gitops`).

- Linguagem: Go 1.25
- Banco: PostgreSQL (`donation_db`)
- Mensageria: Azure Service Bus em produção (`CLOUD_PROVIDER=azure`) ou AWS
  SQS/LocalStack em dev local (default)
- Porta: 8082

## Endpoints

| Método | Rota | Descrição |
|---|---|---|
| GET | `/health` | Health check |
| POST | `/donations` | Cria uma doação (simula aprovação e publica evento na fila) |
| GET | `/donations` | Lista doações |

## Variáveis de ambiente

| Variável | Obrigatória | Descrição |
|---|---|---|
| `PORT` | não (default 8082) | Porta HTTP |
| `DATABASE_URL` | sim | Connection string do PostgreSQL |
| `CLOUD_PROVIDER` | não (default `aws`) | `azure` usa Service Bus; qualquer outro valor tenta SQS |
| `AZURE_SERVICEBUS_CONNECTION_STRING` / `AZURE_SERVICEBUS_QUEUE_NAME` | se `CLOUD_PROVIDER=azure` | Credenciais do Service Bus |
| `AWS_SQS_URL` / `AWS_REGION` / `AWS_ENDPOINT_URL` | não | Fallback SQS (dev local / LocalStack) |
| `OTEL_*` | não | Configuração padrão do OpenTelemetry SDK (ver `fiap-tc-5-gitops/apps/donation`) |

## Observabilidade

Instrumentado com OpenTelemetry (SDK manual, mesmo padrão do
`evaluation-service` da Fase 3/4):

- **Traces**: `otelhttp` no handler raiz + propagação do `traceparent` nas
  propriedades da mensagem de fila, pra o `notification-service` fechar o
  trace distribuído assíncrono.
- **Métricas customizadas**: `solidarytech_http_requests_total` (contador)
  e `solidarytech_http_request_duration_seconds` (histograma) — são a base
  dos 2 SLIs do SLO (taxa de erro e latência p95) no dashboard SRE.

## CI/CD

`.github/workflows/build-push.yaml`: Lint (golangci-lint v2) → Test →
SonarQube (SAST) → Trivy (SCA, ignora CVEs sem fix disponível) → Build/Push
no ACR → atualiza o GitOps (`fiap-tc-5-gitops`).

`.github/workflows/self-heal.yml`: disparado automaticamente pelo Datadog
(via `repository_dispatch`) quando o Monitor de SLO dispara — executa
`kubectl rollout restart` neste serviço.

### Secrets necessários no GitHub

| Secret | Uso |
|---|---|
| `ACR_USERNAME` / `ACR_PASSWORD` | push da imagem |
| `GITOPS_TOKEN` | commit no repo `fiap-tc-5-gitops` |
| `SONAR_TOKEN` / `SONAR_HOST_URL` | SonarQube |
| `AZURE_CREDENTIALS` / `AKS_RESOURCE_GROUP` / `AKS_CLUSTER_NAME` | self-healing |
