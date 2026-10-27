# Auction (Go Expert) - Fechamento automático de leilões

API de leilões em Go (Gin + MongoDB). Quando um leilão é criado, uma goroutine é
agendada para fechá-lo automaticamente ao fim da duração configurada: o status
passa de `Active` para `Completed` no banco, sem intervenção manual. Depois disso,
a validação já existente em `bid` impede novos lances.

## Como funciona

- `AuctionRepository.CreateAuction` (`internal/infra/database/auction/create_auction.go`)
  insere o leilão e, em seguida, dispara `go closeAuctionAfter(...)`. A requisição
  HTTP não espera pelo fechamento.
- A goroutine usa um `time.Timer` até `Timestamp + duração` e então executa um
  `UpdateOne` com filtro `{_id, status: Active}`, o que torna a operação
  idempotente.
- A goroutine usa `context.Background()` com timeout próprio, porque o contexto da
  requisição é cancelado quando o handler responde.

> **Limitação:** os timers vivem na memória do processo. Se a aplicação for
> reiniciada antes de um leilão expirar, ele não será fechado automaticamente.

## Variáveis de ambiente

Definidas em `cmd/auction/.env`:

| Variável | Exemplo | Descrição |
|---|---|---|
| `AUCTION_DURATION` | `20s` | Duração do leilão. Quando expira, o leilão vira `Completed`. |
| `AUCTION_INTERVAL` | `20s` | Usada pela validação de lances para saber se o leilão terminou. Também é o fallback de `AUCTION_DURATION`. |
| `MONGODB_URL` | `mongodb://admin:admin@mongodb:27017/auctions?authSource=admin` | Conexão com o MongoDB |
| `MONGODB_DB` | `auctions` | Nome do banco |

O valor aceita o formato de `time.ParseDuration` (`30s`, `5m`, `1h30m`). Se
`AUCTION_DURATION` estiver ausente, inválida ou não positiva, usa-se
`AUCTION_INTERVAL`; se esta também falhar, o padrão é `5m`.

**Mantenha os dois valores iguais.** O fechamento usa `AUCTION_DURATION`, enquanto a
validação de lances usa `AUCTION_INTERVAL`. Valores diferentes fazem os dois
discordarem sobre o fim do leilão.

## Como rodar

Com Docker Compose (sobe a API e o MongoDB):

```bash
docker compose up --build
```

A API fica em `http://localhost:8080`. Para alterar o tempo, edite
`AUCTION_DURATION` e `AUCTION_INTERVAL` em `cmd/auction/.env` e suba de novo.

Exemplo de uso:

```bash
curl -X POST http://localhost:8080/auction \
  -H "Content-Type: application/json" \
  -d '{"product_name":"Notebook","category":"Electronics","description":"Notebook in perfect condition","condition":1}'

curl "http://localhost:8080/auction?status=1"
```

O parâmetro `status` é obrigatório (`0` = `Active`, `1` = `Completed`). Logo após a
criação o leilão aparece com `"status":0`; depois de `AUCTION_DURATION`, com
`"status":1` e passa a constar na consulta `status=1`.

> No código base, `status=0` é tratado como "sem filtro" e lista todos os leilões.

Localmente (precisa de Go 1.20+ e de um MongoDB acessível):

```bash
go run cmd/auction/main.go
```

Ajuste `MONGODB_URL` no `.env` para `localhost` em vez de `mongodb`.

## Testes

O teste de fechamento cria um leilão com `AUCTION_DURATION=2s`, confirma que ele
está `Active`, aguarda sem tocar no leilão e verifica que passou para `Completed`.
Ele precisa de um MongoDB acessível e é ignorado (`SKIP`) se não houver um.

```bash
docker compose up -d mongodb
go test ./internal/infra/database/auction/... -v
```

Por padrão o teste conecta em
`mongodb://admin:admin@localhost:27017/?authSource=admin` e usa o banco
`auctions_test`, que é removido ao final. Para outro MongoDB, defina
`MONGODB_URL_TEST`.
