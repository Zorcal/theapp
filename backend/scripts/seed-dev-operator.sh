#!/usr/bin/env bash
set -euo pipefail

# Seeds the local dev database with a bootstrap operator user.
#
# Every mutating cmd/cli command requires an existing user for --operator, and there's no seedless
# way to create the very first one — this script exists purely to remove that manual step for local
# development. It talks directly to the theapp-postgres container from infra/docker-compose.yml, so
# it only works there.
#
# Safe to re-run: the insert is a no-op if the operator already exists.

docker exec theapp-postgres psql -U postgres -d theapp -v ON_ERROR_STOP=1 -c "
  insert into useraccess.users (external_id, email, name, created_at, etag)
  values (gen_random_uuid(), 'operator@theapp.com', 'Operator', now(), gen_random_uuid())
  on conflict (email) do nothing;
"
