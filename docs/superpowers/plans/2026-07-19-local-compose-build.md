# Local Docker Compose Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a standalone Compose stack that builds the complete new-api image from the local working tree and runs it with healthy MySQL and Redis dependencies.

**Architecture:** A root-level `docker-compose.local.yml` will build the existing multi-stage `Dockerfile`, publish the application on a configurable local port, and connect it to MySQL and Redis over Compose's private default network. Host bind mounts preserve application data and logs, while a named volume preserves MySQL data.

**Tech Stack:** Docker Engine, Docker Compose v2, the repository's multi-stage Dockerfile, MySQL 8.2, Redis 7 Alpine

---

### Task 1: Add The Local Compose Stack

**Files:**
- Create: `docker-compose.local.yml`

- [x] **Step 1: Verify the configuration does not exist yet**

Run:

```powershell
docker compose -f docker-compose.local.yml config --quiet
```

Expected: FAIL because `docker-compose.local.yml` does not exist.

- [x] **Step 2: Create the complete local stack**

Create `docker-compose.local.yml` with this content:

```yaml
# Local full-stack build. Defaults are intended for local development only.
# Build and start: docker compose -f docker-compose.local.yml up -d --build

name: new-api-local

services:
  new-api:
    build:
      context: .
      dockerfile: Dockerfile
    image: new-api:local
    restart: unless-stopped
    command: ["--log-dir", "/app/logs"]
    ports:
      - "127.0.0.1:${NEW_API_PORT:-3000}:3000"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    environment:
      SQL_DSN: "root:${MYSQL_ROOT_PASSWORD:-new-api-local}@tcp(mysql:3306)/new-api?charset=utf8mb4&parseTime=true&loc=Local"
      REDIS_CONN_STRING: redis://redis:6379
      TZ: Asia/Shanghai
      ERROR_LOG_ENABLED: "true"
      BATCH_UPDATE_ENABLED: "true"
      NODE_NAME: new-api-local-node
      SESSION_SECRET: "${SESSION_SECRET:-}"
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O - http://localhost:3000/api/status | grep -q '\"success\":[[:space:]]*true'"]
      interval: 10s
      timeout: 5s
      retries: 12
      start_period: 30s

  mysql:
    image: mysql:8.2
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: "${MYSQL_ROOT_PASSWORD:-new-api-local}"
      MYSQL_DATABASE: new-api
      TZ: Asia/Shanghai
    command:
      - --character-set-server=utf8mb4
      - --collation-server=utf8mb4_unicode_ci
      - --default-authentication-plugin=caching_sha2_password
    volumes:
      - local_mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysql --protocol=TCP -h 127.0.0.1 -uroot -p\"$${MYSQL_ROOT_PASSWORD}\" -e \"SELECT 1\""]
      interval: 10s
      timeout: 5s
      retries: 12

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    command: ["redis-server", "--appendonly", "yes"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  local_mysql_data:
```

- [x] **Step 3: Validate the rendered Compose model**

Run:

```powershell
docker compose -f docker-compose.local.yml config --quiet
```

Expected: PASS with exit code 0 and no output.

- [x] **Step 4: Inspect the rendered build and dependency settings**

Run:

```powershell
docker compose -f docker-compose.local.yml config
```

Expected: the `new-api` service contains `build.context`, image `new-api:local`, loopback-only port `3000`, both bind mounts, and healthy dependency conditions for `mysql` and `redis`.

- [x] **Step 5: Commit the Compose configuration**

```powershell
git add docker-compose.local.yml
git commit -m "chore: add local compose build"
```

### Task 2: Build And Verify The Running Stack

**Files:**
- Verify: `docker-compose.local.yml`
- Verify: `Dockerfile`

- [x] **Step 1: Build the complete application image**

Run:

```powershell
docker compose -f docker-compose.local.yml build new-api
```

Expected: Docker completes the frontend and Go build stages and tags the final image as `new-api:local`.

- [x] **Step 2: Start the stack**

Run:

```powershell
docker compose -f docker-compose.local.yml up -d
```

Expected: Compose starts `mysql`, `redis`, and `new-api`; the application starts after both dependencies report healthy.

- [x] **Step 3: Check service health**

Run:

```powershell
docker compose -f docker-compose.local.yml ps
```

Expected: all three services are running, and services with health checks report `healthy` after startup completes.

- [x] **Step 4: Verify the public application endpoint**

Run:

```powershell
$response = Invoke-RestMethod -Uri http://localhost:3000/api/status
if (-not $response.success) { throw "new-api health check failed" }
$response.success
```

Expected: `True`.

- [x] **Step 5: Confirm the local image exists**

Run:

```powershell
docker image inspect new-api:local --format '{{.RepoTags}}'
```

Expected: output contains `new-api:local`.

- [x] **Step 6: Leave the verified stack running for local use**

The application remains available at `http://localhost:3000`. To stop it later, run:

```powershell
docker compose -f docker-compose.local.yml down
```
