#!/usr/bin/env bash
set -euo pipefail

# Seeds the local dev database with a bootstrap operator user, granted superadmin at system scope.
#
# Every mutating cmd/cli command requires an existing user for --operator, and there's no seedless
# way to create the very first one — this script exists purely to remove that manual step for local
# development. It talks directly to the theapp-postgres container from infra/docker-compose.yml, so
# it only works there. It grants superadmin with a direct SQL insert rather than a CLI command,
# since no such command exists yet.
#
# Safe to re-run: both inserts are no-ops if already applied.

docker exec theapp-postgres psql -U postgres -d theapp -v ON_ERROR_STOP=1 -c "
  insert into useraccess.users (external_id, email, name, created_at, etag)
  values (gen_random_uuid(), 'operator@theapp.com', 'Operator', now(), gen_random_uuid())
  on conflict (email) do nothing;

  insert into rbac.system_role_assignments (user_id, role_id)
  select u.id, r.id
  from useraccess.users u
  cross join rbac.roles r
  where u.email = 'operator@theapp.com' and r.name = 'superadmin'
  on conflict do nothing;
"
