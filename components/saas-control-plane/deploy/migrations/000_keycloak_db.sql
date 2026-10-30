-- Creates the database Keycloak stores its realm data in.
--
-- Keycloak previously ran with `start-dev` on an embedded H2 database held inside
-- the container, which meant every user, group, role mapping, and identity
-- provider created through the Admin Portal was lost whenever the container was
-- recreated. Giving Keycloak its own database on the existing PostgreSQL instance
-- makes that state durable.
--
-- This runs from /docker-entrypoint-initdb.d, which the postgres image only
-- executes when initialising an EMPTY data directory. Existing deployments must
-- create the database by hand:
--
--   docker compose exec -T postgres psql -U clm -d clm_db \
--     -c "CREATE DATABASE keycloak OWNER clm;"
--
-- The filename is prefixed 000 so it runs before the CLM schema migrations.

CREATE DATABASE keycloak OWNER clm;
