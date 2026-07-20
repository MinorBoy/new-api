# Local Docker Compose Build Design

## Goal

Add a standalone Docker Compose configuration that builds the complete new-api application from the local working tree and runs it with MySQL and Redis.

## Configuration

Create `docker-compose.local.yml` at the repository root. It will define three services:

- `new-api` builds the root `Dockerfile`, tags the result as `new-api:local`, publishes port `3000` on host loopback only, and mounts `./data` and `./logs` for application persistence and inspection.
- `mysql` uses MySQL 8.2 with `utf8mb4` and stores its database in a named volume.
- `redis` uses Redis 7 and provides the application cache.

Compose-generated service names will be used instead of fixed `container_name` values so this stack can coexist with the repository's other Compose configurations.

## Data Flow

The browser or API client connects to `localhost:3000`. The application connects to MySQL and Redis through Compose DNS names on the stack's private default network. MySQL data survives container recreation through a named volume; application data and logs remain available in the repository's existing host directories.

## Startup And Failure Handling

MySQL and Redis will expose health checks. The application will wait for both dependencies to become healthy before starting and will have its own HTTP health check against `/api/status`. Services will use `unless-stopped` restart behavior for local convenience.

Local defaults will make the stack runnable without an extra environment file. Passwords and other settings will support Compose environment-variable overrides, and comments will state that the defaults are for local development only. When `SESSION_SECRET` is not provided, the application keeps its randomly generated default instead of using a repository-known key.

## Usage

Build and start the stack with:

```sh
docker compose -f docker-compose.local.yml up -d --build
```

Stop it with:

```sh
docker compose -f docker-compose.local.yml down
```

Remove containers and database volumes with:

```sh
docker compose -f docker-compose.local.yml down -v
```

## Verification

Validate the rendered configuration with `docker compose config`, build the local application image, start the services, and confirm the application health endpoint succeeds on `http://localhost:3000/api/status`.
