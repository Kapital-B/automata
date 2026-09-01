# POC Release Gates

This checklist is the release gate for the AWS-hosted Automata POC across Floci local, `dev`, and `prod`.

## Before `dev`

- Verify Floci local smoke flow still works end to end:
  - `svc/terraform/envs/local` applies against Floci with `aws_endpoint` overrides intact.
  - API Lambda responds through the local API Gateway URL.
  - Scheduler and worker Lambdas can process DynamoDB stream-driven jobs.
  - Local Postgres remains the product database for `environment = "local"`.
- Confirm the four bootstrap Lambdas are built and wired correctly:
  - `api`
  - `scheduler`
  - `worker`
  - `migrate`
- Confirm the DynamoDB jobs table uses `expires_at` TTL and `JOB_TERMINAL_RETENTION_DAYS = 30`.

## Hosted `dev` checks

- Apply `svc/terraform/envs/dev` in the backend account (API region).
- Apply `web/terraform/envs/dev` in the frontend account (`us-east-1`).
- From the **root** account zone `kapital-b.com`, NS-delegate:
  - `automata-dev` → `terraform output hosted_zone_name_servers` from **web** (SPA apex)
- From the **SPA apex zone** in the frontend account (`automata-dev.kapital-b.com`), NS-delegate:
  - `api` → `terraform output hosted_zone_name_servers` from **svc** (API)
  - Do **not** rely only on an `api.automata-dev` NS cut on `kapital-b.com`: `api.*` is hierarchically under the SPA apex, so resolvers that follow that apex often NXDOMAIN the API host and never hit API Gateway/Lambda (browser shows a CORS-looking failure with empty Lambda logs).
- Validate the API custom domain:
  - `api.automata-dev.kapital-b.com`
  - ACM certificate issued in the API region
  - API Gateway base path mapping resolves through the custom domain
- Validate the SPA custom domain:
  - `automata-dev.kapital-b.com`
  - CloudFront distribution serves the built SPA
  - `VITE_API_BASE_URL` points at `https://api.automata-dev.kapital-b.com`
- Confirm DSQL wiring:
  - hosted service uses Aurora DSQL, not a local or static Postgres endpoint
  - runtime Lambdas have `dsql:DbConnect`
  - migrate Lambda alone has `dsql:DbConnectAdmin`
- Confirm Bedrock access:
  - API and worker Lambdas can invoke `eu.amazon.nova-2-lite-v1:0`
  - IAM allows the inference profile and backing foundation model resource
- Confirm runtime configuration:
  - OAuth redirect URIs match the hosted API domain
  - finite CloudWatch log retention is set
  - no AWS Backup, PITR, alarms, dashboards, or SNS resources are present
- Populate Secrets Manager values from `terraform output secret_names` (empty secrets are created by apply; Terraform does not set values):
  - `automata/dev/ENCRYPTION_KEY` (exactly 32 bytes)
  - `automata/dev/JWT_SECRET` (>=32 bytes)
  - `automata/dev/JOB_CURSOR_SECRET` (>=32 bytes; may match JWT)
  - `automata/dev/MS_CLIENT_SECRET`
  - `automata/dev/GOOGLE_CLIENT_SECRET` / `SLACK_CLIENT_SECRET` when those providers are enabled
- Run E2E smoke coverage on `automata-dev.kapital-b.com`:
  - auth flow
  - account connection flow
  - one async job that reaches `success`
  - one forced failure that surfaces in logs and the stream-failure bucket

## Failure matrix

- DNS not delegated: API and SPA ACM validation must remain pending until both NS delegations are fixed.
- DSQL permission mismatch: runtime should fail with `DbConnect`; migrations should fail with `DbConnectAdmin`.
- Bedrock permission mismatch: API and worker should fail only on model invocation paths.
- DynamoDB stream failure: failed records should land in the stream-failure bucket and not block unrelated requests.
- OAuth misconfiguration: callback failures should remain isolated to the provider being exercised.

## Promotion to `prod`

- Repeat the hosted checks in `svc/terraform/envs/prod` and `web/terraform/envs/prod`.
- From `kapital-b.com`, NS-delegate:
  - `automata` → web `hosted_zone_name_servers`
- From the SPA apex zone (`automata.kapital-b.com`), NS-delegate:
  - `api` → svc `hosted_zone_name_servers`
- Confirm `api.automata.kapital-b.com` and `automata.kapital-b.com` behave identically to `dev`.
- Promote only after `dev` E2E coverage, DSQL/IAM checks, and Bedrock checks are all green.

## POC deferrals

- Secrets Manager secret *containers* are created by Terraform; secret *values* are populated manually after apply (never via tfvars).
- Backups, PITR, alarms, dashboards, and SNS are explicitly deferred and must be added before a non-POC production hardening pass.
