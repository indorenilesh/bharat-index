# Bharat Index

Bharat Index is a stock index simulation built from independently buildable Go microservices. Stock services generate a price tick for each configured share every second. The index calculator averages the latest price for each share, records the result, and makes it available to a live web dashboard.

The initial shares are BATA and TATA. Their price ranges are stored in PostgreSQL and can be changed through the admin console.

## Source layout

```text
db/
  Dockerfile                 PostgreSQL image and first-run schema
  schema.sql
services/
  stock/                     Standalone stock tick service module and image
    Dockerfile
    go.mod
    go.sum
    main.go
  calc-index/                Standalone index calculation module and image
    Dockerfile
    go.mod
    go.sum
    main.go
  ui/                        Standalone dashboard module, image, and page
    Dockerfile
    go.mod
    go.sum
    main.go
    ui.html
  admin/                     Standalone range-admin module, image, and page
    Dockerfile
    go.mod
    go.sum
    main.go
    admin.html
compose.yaml                 Local multi-service orchestration
```

Each application service has its own Go module, pinned dependencies, Dockerfile, and Docker build context. Building one service does not compile or copy the source of the other application services. The BATA and TATA processes use the same stock-service image and run as separate containers, configured with different stock IDs.

### Stock service instances

There is one stock-service image, but each stock runs in its own container/process with a unique `STOCK_ID`. For example, Compose runs two instances of `bharat-index-stock:local`: one with `STOCK_ID=BT001` for BATA and another with `STOCK_ID=TT001` for TATA. Build the image once and use that image for every stock; adding a stock does not require a new image.

The current Compose file explicitly configures BATA and TATA. It does not automatically create a container for every row in `s_detail`. For more stocks, add one service instance per stock or generate the Compose/orchestration configuration from stock definitions. In ECS or Kubernetes, configure one task or pod per stock and give each a distinct `STOCK_ID`. Do not run duplicate instances with the same ID unless duplicate tick generation is intended.

The index calculator must also be configured with all the stock IDs it should include through `STOCK_IDS` (for example, `BT001,TT001`). Creating stock containers alone does not automatically add their IDs to the index calculation.

## Microservices

| Service | Description |
| --- | --- |
| `postgres` | PostgreSQL database. On first initialization, it creates the environment-prefixed database (by default `dev-bharat-index`) and loads the schema and initial index and share records from `db/schema.sql`. |
| `redis` | Redis cache for the latest price tick for each share and the latest calculated index value. |
| `bata` | Stock-service container configured for BATA (`BT001`). Generates a random price each second using the current range in `s_detail`, then writes the tick to PostgreSQL and Redis. |
| `tata` | Stock-service container configured for TATA (`TT001`). Generates a random price each second using the current range in `s_detail`, then writes the tick to PostgreSQL and Redis. |
| `calc-index` | Reads the latest share prices from Redis, averages them once per second, and stores each index tick in PostgreSQL and Redis. |
| `ui` | Live Bharat Index dashboard. Displays the current value, movement, recent high and low, and a chart of the last 120 ticks. |
| `admin` | Login-free console for viewing and updating share price ranges in `s_detail`. The share services reload their range every second and apply changes on their next tick. |

## Run the project

### Prerequisites

- Docker Desktop with Docker Compose
- PowerShell (commands below are for Windows)

### Start

From the repository root, set a PostgreSQL password and start all services:

```powershell
$env:POSTGRES_PASSWORD = 'choose-a-strong-local-password'
docker compose up --build
```

Keep this terminal open while running the project. To keep the password between terminal sessions, create a `.env` file next to `compose.yaml`:

```dotenv
POSTGRES_PASSWORD=choose-a-strong-local-password
```

Do not commit `.env` or use a development password in production.

When the services are ready, open:

- **Live index dashboard:** [http://localhost:8080](http://localhost:8080)
- **Share range admin:** [http://localhost:8081](http://localhost:8081)

The admin console does not have authentication. Its port is bound to localhost for local development; do not expose it to an untrusted network.

### Stop

Press **Ctrl+C** in the terminal running Compose, or stop the services from another terminal:

```powershell
docker compose down
```

PostgreSQL and Redis data are kept in named Docker volumes when the containers stop. To remove the volumes and all persisted data as well:

```powershell
docker compose down --volumes
```

The schema initialization SQL runs only when PostgreSQL initializes an empty data directory. If you change `db/schema.sql` after the database volume has already been created, those changes will not be applied automatically.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `POSTGRES_PASSWORD` | Required | Password for the PostgreSQL user. |
| `POSTGRES_USER` | `bharatindex` | PostgreSQL user shared by the database and application services. |
| `DB_ENV_PREFIX` | `dev` | Prefix for the database name. For example, `staging` uses `staging-bharat-index`. |
| `STOCK_IDS` | `BT001,TT001` | Comma-separated stock IDs used in the index calculation. |

In PowerShell, set optional environment values before starting Compose, for example:

```powershell
$env:DB_ENV_PREFIX = 'staging'
$env:POSTGRES_PASSWORD = 'choose-a-strong-local-password'
docker compose up --build
```

PostgreSQL is published on `localhost:5432`. Redis is available to other Compose services on the internal network and is not published to the host.

## Build an application image independently

Run these commands from the repository root. Each command uses only that service's directory as its Docker build context:

```powershell
docker build -t bharat-index-stock:local .\services\stock
docker build -t bharat-index-calc-index:local .\services\calc-index
docker build -t bharat-index-ui:local .\services\ui
docker build -t bharat-index-admin:local .\services\admin
```

The stock image is run once for each share by Compose. To run it directly, configure `STOCK_ID` as `BT001` or `TT001` and provide its Postgres and Redis connection environment variables. You can rebuild and deploy one service without rebuilding the others.

The PostgreSQL image can also be built separately; Redis uses the upstream image directly:

```powershell
docker build -t bharat-index-postgres:local .\db
```

## AWS ECS and Kubernetes

The application images are standalone and consume connection settings from environment variables rather than requiring Docker Compose. Build and push each image to a registry accessible to the target runtime, then deploy each service as its own ECS task/service or Kubernetes Deployment:

- `bharat-index-stock`: deploy two instances/tasks with `STOCK_ID=BT001` and `STOCK_ID=TT001`.
- `bharat-index-calc-index`: configure `STOCK_IDS` (defaults to `BT001,TT001`).
- `bharat-index-ui`: expose container port `8080`.
- `bharat-index-admin`: expose container port `8081` only on a trusted/private network. This console has no authentication.

Provide `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, and `PGDATABASE` to services that use PostgreSQL. `PGSSLMODE` controls the PostgreSQL TLS mode; it defaults to `disable` for local Compose, and should be set according to the database provider's requirements (for example, `require` for a TLS connection). The stock and index services also need `REDIS_ADDR`. In cloud deployments, point these variables to reachable managed or separately deployed PostgreSQL and Redis endpoints; the Compose hostnames `postgres` and `redis` only resolve inside the Compose network. Supply passwords through the platform's secret manager, not source control or image build arguments.

The service source and image builds are isolated, but the services still share PostgreSQL tables and Redis key conventions as integration contracts. Keep schema changes backward-compatible while consumers are updated. For stronger data ownership and independent schema evolution, a future step is to let each service own its persistence and expose versioned APIs or events to other services.

For example, tag each independently built image for your registry and push it:

```powershell
$registry = 'your-registry.example'
$tag = 'latest'

docker build -t "$registry/bharat-index-stock:$tag" .\services\stock
docker build -t "$registry/bharat-index-calc-index:$tag" .\services\calc-index
docker build -t "$registry/bharat-index-ui:$tag" .\services\ui
docker build -t "$registry/bharat-index-admin:$tag" .\services\admin
docker push "$registry/bharat-index-stock:$tag"
docker push "$registry/bharat-index-calc-index:$tag"
docker push "$registry/bharat-index-ui:$tag"
docker push "$registry/bharat-index-admin:$tag"
```

Infrastructure provisioning, image publishing, and platform deployment manifests are deliberately kept separate from the application images. Add ECS task definitions or Kubernetes manifests/Helm charts for the environment where you deploy.
