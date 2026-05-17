# Repo Automation Reference

This directory hosts the repo-level Moon project (`repo`) plus the localmock bootstrap helper script.

## Common Moon Tasks

Run these from the repository root:

```bash
moon run :help
moon run :build
moon run :test
moon run :lint
moon run :format
moon run :clean
moon run :local-up
moon run :local-down
moon run :local-bootstrap
```

## Localmock Bootstrap Script

`bootstrap-localmock.sh` performs the full localmock bootstrap flow:

- starts Kafka first
- applies Kafka Terraform topics and ACLs
- starts the remaining Docker Compose services
- waits for Keycloak readiness
- applies Keycloak Terraform configuration

Use it through Moon so the required `direnv` environment is loaded:

```bash
moon run :local-bootstrap
```

## Notes

- The repo-level helper tasks in `scripts/moon.yml` now exist primarily for `:help`, `:clean`, and `:dev`; build/test/lint/format use the generic Moon `:task` entrypoints.
- `localmock` Moon tasks wrap `direnv exec localmock ...` so local secrets are available automatically.
