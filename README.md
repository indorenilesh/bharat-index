# Bharat Index

Bharat Index is a local, Docker Compose-based stock index simulation. Independent services generate a price tick for each configured share every second. The index calculator averages the latest price for each share, records the result, and makes it available to a live web dashboard.

The initial shares are BATA and TATA. Their price ranges are stored in PostgreSQL and can be changed through the admin console.

## Microservices

| Service | Description |
| --- | --- |
| `postgres` | PostgreSQL database. On first initialization, it creates the environment-prefixed database (by default `dev-bharat-index`) and loads the schema and initial index and share records from `db/schema.sql`. |
| `redis` | Redis cache for the latest price tick for each share and the latest calculated index value. |
| `bata` | Generates a random BATA (`BT001`) price each second, using the current range in `s_detail`, and writes each tick to PostgreSQL and Redis. |
| `tata` | Generates a random TATA (`TT001`) price each second, using the current range in `s_detail`, and writes each tick to PostgreSQL and Redis. |
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
