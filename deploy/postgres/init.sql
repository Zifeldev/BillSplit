-- One Postgres login role per microservice (least privilege).
--
-- Runs once, on the first start of an EMPTY data volume (docker-entrypoint-initdb.d),
-- as the bootstrap superuser (POSTGRES_USER) connected to POSTGRES_DB. After
-- changing this file locally, recreate the volume: `docker compose down -v`.
--
-- Model:
--   * each service role owns exactly one schema (CREATE SCHEMA ... AUTHORIZATION),
--     so it has full DDL/DML there (goose migrations, goose_db_version table);
--   * no role gets USAGE on any other service schema, so it cannot even resolve
--     another service's tables ("permission denied for schema ...");
--   * the public schema, TEMPORARY and CREATE on the database are revoked from
--     PUBLIC, so nobody can put objects into public or temp schemas;
--   * service roles get only CONNECT on the database (no CREATE): their schemas
--     are created here, and postgres.Migrate skips CREATE SCHEMA when it exists;
--   * the superuser (POSTGRES_USER) is for admin/backup only, never for services.
--
-- Roles are created without passwords here; 20-role-passwords.sh sets them from
-- Docker secrets (secrets/<service>_db_password). No password lives in SQL or
-- compose files.

\set ON_ERROR_STOP on

CREATE ROLE auth_svc LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
CREATE ROLE social_svc LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
CREATE ROLE split_svc LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
CREATE ROLE finance_svc LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
CREATE ROLE notify_svc LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;

-- Lock down defaults inherited by every role through PUBLIC.
REVOKE ALL ON SCHEMA public FROM PUBLIC;
SELECT format('REVOKE CREATE, TEMPORARY, CONNECT ON DATABASE %I FROM PUBLIC', current_database()) \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO auth_svc, social_svc, split_svc, finance_svc, notify_svc', current_database()) \gexec

-- One schema per service, owned by its role.
CREATE SCHEMA auth    AUTHORIZATION auth_svc;
CREATE SCHEMA social  AUTHORIZATION social_svc;
CREATE SCHEMA split   AUTHORIZATION split_svc;
CREATE SCHEMA finance AUTHORIZATION finance_svc;
CREATE SCHEMA notify  AUTHORIZATION notify_svc;

-- Pin search_path at the role level too (the app also sets it per connection).
ALTER ROLE auth_svc    SET search_path = auth;
ALTER ROLE social_svc  SET search_path = social;
ALTER ROLE split_svc   SET search_path = split;
ALTER ROLE finance_svc SET search_path = finance;
ALTER ROLE notify_svc  SET search_path = notify;
