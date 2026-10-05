# Operations: CLI, plans, domains and config

For whoever runs the servers: the command-line tools, plan limits, custom domains, how each request is routed, and the configuration file.

## 3. Operator command-line tools

Run them on the server with the binary, for example `./conectdesk --config config.toml <flag>`. Each command does its job and exits; the server doesn't start.

The binary checks things in this order:

1. `--version`
2. Config is loaded
3. `--install`
4. `--set-system-user-password`
5. A check that the database schema exists ("database tables are missing. Use the `--install` flag…")
6. `--upgrade`
7. A check for pending upgrades: the binary refuses to run until you run `--upgrade`
8. `--copy-uploads-to-s3`
9. `--platform-admin-*`, then `--docs-viewer-*`
10. `--tenant-*` / `--seed-demo`

Only one command runs per invocation. Within each group the first match wins.

### General flags

| Flag | What it does | Example |
|---|---|---|
| `--config <file>` | One or more config files, merged in order. Default `config.toml`. If the file is missing, it continues with defaults and environment variables. | `--config config.toml --config secrets.toml` |
| `--version` | Prints the build string and exits. | `./conectdesk --version` |
| `--yes` | Skips confirmation prompts for `--install`, `--upgrade` and `--tenant-delete`. | |
| `--static-dir <dir>` | Uses a directory of custom static files and templates that override the built-in ones (same layout as `static/`). It fails if the path isn't accessible. | `--static-dir /opt/brand` |

### Database setup and upgrade

| Flag | What it does | Example |
|---|---|---|
| `--install` | First-time setup. It warns "WARNING: This will wipe your entire database - '<db>'", asks "Continue (y/n)?" (unless `--yes`), waits 5 seconds, installs the schema, creates the default workspace's System user, and marks all migrations as applied. The System user's password comes from the `CONECTDESK_SYSTEM_USER_PASSWORD` environment variable (it must be strong), otherwise a random one. | `./conectdesk --install` |
| `--idempotent-install` | Used with `--install`. Skips everything if the schema is already installed, which is safe for container start scripts. | `./conectdesk --install --idempotent-install --yes` |
| `--upgrade` | Runs pending database migrations. It prints "** IMPORTANT: Take a backup of the database before upgrading." and asks "continue (y/n)?" (unless `--yes`). If there's nothing to do, it prints "no upgrades to run. Database is up to date." | `./conectdesk --upgrade --yes` |
| `--set-system-user-password` | Prompts for a new password for the default workspace's System user. Afterwards, log in with email "System". | `./conectdesk --set-system-user-password` |
| `--copy-uploads-to-s3` | Copies files from `upload.fs.upload_path` into the S3 bucket in `[upload.s3]`, skipping files already there, so it can be re-run. Use it when moving from `fs` to `s3`. Afterwards set `upload.provider = "s3"`. | `./conectdesk --copy-uploads-to-s3` |

### Platform admins

The password is read from the terminal without echo. If input is piped in, it is read from stdin.

| Flag | What it does | Example |
|---|---|---|
| `--platform-admin-add <email>` | Creates a platform admin, or **resets the password** of an existing one. It asks for the password, and asks you to repeat it in a terminal. The password must be 10 to 72 characters with upper, lower, a number and a symbol. Prints "platform admin "x" can log in at the platform dashboard". | `./conectdesk --platform-admin-add ops@yourapp.com` |
| `--platform-admin-remove <email>` | Deletes the admin and ends their sessions. | `./conectdesk --platform-admin-remove old@yourapp.com` |
| `--platform-admin-list` | A table of ID, EMAIL, CREATED, LAST LOGIN and 2FA (true/false). | `./conectdesk --platform-admin-list` |
| `--platform-admin-reset-2fa <email>` | Turns off the admin's two-factor login, for a lost phone. They set it up again at their next login. | `./conectdesk --platform-admin-reset-2fa ops@yourapp.com` |

### Docs viewers

Accounts that can only read this documentation (see Platform admin → Docs access). The password is read as for `--platform-admin-add`.

| Flag | What it does | Example |
|---|---|---|
| `--docs-viewer-add <email>` | Creates a docs viewer, or **resets the password** of an existing one. Same password rules as platform admins. Prints "docs viewer "x" can log in to the documentation". | `./conectdesk --docs-viewer-add teammate@arrlink.com` |
| `--docs-viewer-remove <email>` | Deletes the viewer and ends their sessions. | `./conectdesk --docs-viewer-remove teammate@arrlink.com` |
| `--docs-viewer-list` | A table of ID, EMAIL, CREATED and LAST LOGIN. | `./conectdesk --docs-viewer-list` |

On Railway and with docker-compose, set `DOCS_VIEWER_EMAIL` and `DOCS_VIEWER_PASSWORD` to create that docs viewer (or reset its password) on every deploy, the same way `PLATFORM_ADMIN_EMAIL` / `PLATFORM_ADMIN_PASSWORD` create the platform admin. A rejected one is logged and the server starts anyway.

### Workspaces (tenants)

| Flag | What it does | Example |
|---|---|---|
| `--tenant-list` | A table of ID, SLUG, NAME, PLAN, STATUS, CREATED and USAGE (agents, inboxes). | `./conectdesk --tenant-list` |
| `--tenant-create <slug>` plus `--tenant-name`, `--tenant-admin-name`, `--tenant-admin-email` | Creates a workspace and its admin. The admin password is prompted for, or read from stdin. Needs `app.tenancy.enabled`. It uses `default_plan` and allows **all** channels and integrations. The first word of `--tenant-admin-name` becomes the first name and the rest the last name. It applies the same validation and reserved slugs as the dashboard. | `echo 'S3cure!Pass1' \| ./conectdesk --tenant-create acme --tenant-name "Acme Tuition" --tenant-admin-name "Jane Doe" --tenant-admin-email jane@acme.com` |
| `--tenant-suspend <slug>` | Suspends the workspace. It refuses for the default workspace. | `./conectdesk --tenant-suspend acme` |
| `--tenant-activate <slug>` | Sets the status to active. **Caution:** unlike the dashboard, it doesn't check for a scheduled deletion. A workspace scheduled for deletion becomes active but is **still deleted** when its date arrives. Use `--tenant-restore` for those. | `./conectdesk --tenant-activate acme` |
| `--tenant-restore <slug>` | Cancels a scheduled deletion and activates the workspace. It fails with "isn't scheduled for deletion" otherwise. | `./conectdesk --tenant-restore acme` |
| `--tenant-delete <slug>` | Deletes the workspace and **all** its data and stored files **immediately**, with no grace period. It asks you to type the slug to confirm (unless `--yes`). It refuses for the default workspace. | `./conectdesk --tenant-delete acme` |
| `--seed-demo <slug>` | Fills a workspace that has **no conversations** with demo data for a coaching institute: contacts, conversations and call-backs. It prints the counts. | `./conectdesk --seed-demo demo` |

### Environment variables

Any config key can be set with `CONECTDESK_` followed by the key in uppercase, with `__` for each dot. For example, `CONECTDESK_APP__TENANCY__BASE_DOMAIN=yourapp.com`. The legacy prefix `LIBREDESK_` is still read, and `CONECTDESK_` overrides it.

---

## 4. Plans, limits, domains and routing

### 4.1 Plans and limits

**Where they are defined:** `[app.tenancy.plans.<name>]` in the config file. The plan names offered in the dashboard are exactly these section names, sorted alphabetically. New workspaces start on `app.tenancy.default_plan` (default `free`).

**Who is unlimited:**

- Any limit set to `0`, or missing
- A plan that isn't listed in the config
- The default workspace (ID 1)
- Single-tenant mode

| Key | Meaning | Where it is enforced | Message the workspace sees |
|---|---|---|---|
| `max_agents` | Agent count | Creating an agent, and agent CSV import | "Your plan allows {max} agents. Upgrade your plan to add more" |
| `max_inboxes` | Inbox count | Creating an inbox, Gmail/Outlook OAuth connection, Meta connection, Telegram connection | "Your plan allows {max} inboxes. Upgrade your plan to add more" |
| `max_storage_mb` | Total stored files (attachments, avatars) | Media upload, live chat widget upload, avatar upload | "Your plan's {max} of file storage is full. Delete files or upgrade your plan" |
| `max_file_upload_mb` | Largest single file. It caps the workspace's own "max upload size" setting. | Through the workspace's upload limit | (the normal upload size error) |
| `api_requests_per_minute` | All signed-in requests of the whole workspace (every agent and API key together), per minute | Every signed-in request | HTTP 429 (too many requests). Each refusal counts toward "Throttled today". |
| `ai_requests_per_month` | AI replies, rewrites, summaries and AI agent runs. Search embeddings are not counted. | Every AI completion and agent run | "Your plan's {max} AI requests this month are used up. Upgrade your plan for more" |

Sample plans in `config.sample.toml`:

| Plan | Agents | Inboxes | Storage | Largest upload | API requests/min | AI requests/month |
|---|---|---|---|---|---|---|
| **free** | 2 | 1 | 1024 MB | 10 MB | 300 | 200 |
| **growth** | 15 | 5 | 20480 MB | 25 MB | 1200 | 5000 |
| **pro** | unlimited | unlimited | unlimited | unlimited | unlimited | unlimited |

Workspace admins see their own usage under **Admin → Usage**. If `app.tenancy.upgrade_url` is set, that page has an upgrade button linking to it, which can be a pricing page or a `mailto:` link.

Changing a plan's numbers in the config takes effect after a restart. Changing a workspace's plan in the dashboard takes effect immediately.

### 4.2 Custom domains

**Where:** Inside the workspace, **Admin → Domains** (`/admin/domains`).

**Who can use it:** Workspace users with the `general_settings:manage` permission.

**When it exists:** Only with multi-tenancy on. Otherwise the API replies "Custom domains are only available on multi-workspace installations."

**Purpose:** Serve the workspace and/or its help centres on the customer's own domain, for example `support.example.com`.

**The flow:**

1. **Add domain.** The workspace admin types the hostname and presses **Add domain**. A URL is accepted, and its scheme and path are removed.
   - Refused: IP addresses, single-word names, invalid labels, and the base domain or any of its subdomains. Message: "Enter a domain name you own, such as support.example.com, without https:// or a path."
   - Already added by this workspace: "This domain is already added."
   - Verified by another workspace: "Another workspace has verified this domain."
2. **Prove ownership** by adding the TXT record shown on screen:
   - **Name:** `_conectdesk.<hostname>`
   - **Value:** `conectdesk-verification=<token>`
   - Records named with the older `_libredesk.` prefix are also accepted.
3. **Point the domain** at the workspace with a CNAME from `<hostname>` to `<slug>.<base_domain>`.
4. Press **Verify**. The server looks up the TXT record, using `app.tenancy.dns_server` if set, with a 10-second timeout.
   - If the record is missing or wrong, the domain stays "Not verified" and the reason is shown: "No TXT record found at …", "The TXT record at … doesn't have the value …", or "Looking up … failed, try again in a few minutes."
   - On success: "<domain> is verified."
5. **Use as workspace address** (only for verified domains) makes it the primary address, used in links and emails. **Use <slug>.<base_domain> instead** switches back to the subdomain. A verified domain can also be set as a help centre's custom domain.
6. **Delete** asks for confirmation: "<domain> will stop serving your workspace and help centers."
   - It is refused while a help centre uses the domain: "A help center is served on this domain. Remove the domain from the help center first."
   - Removing the primary domain makes the subdomain the address again.

**Automatic re-checks:** Every `app.tenancy.domain_recheck_interval` (default 24h), every verified domain's TXT record is checked again.

- If the record is missing in a check, the domain shows a warning: "The verification record is missing (N of 3 daily checks). Put it back, or the domain stops working."
- After **3 failed checks in a row** the domain is unverified. It stops routing, and if it was the workspace address, the address falls back to the subdomain.
- A lookup that fails outright (rather than finding no record) doesn't count as a failure.

**Platform view:** Platform admins see a workspace's domains, read-only, on its detail page (1.6).

### 4.3 TLS "ask" endpoint (for on-demand certificates)

**Where:** `GET /tls/ask?domain=<hostname>`. It answers on any host and is handled before any workspace routing.

**Who uses it:** Your TLS proxy, for example Caddy's `on_demand_tls { ask http://app:9000/tls/ask }`, so it only requests certificates for hosts the installation actually serves.

**Answers:**

| Hostname | Response |
|---|---|
| The base domain | **200** |
| `<slug>.<base_domain>` where the workspace exists, whatever its status | **200** |
| A custom domain that is verified by any workspace | **200** |
| Anything else, or an empty `domain` | **404** |
| A database error | **500** |

Note that `admin.<base_domain>` and `docs.<base_domain>` are not workspace slugs, so the ask endpoint returns 404 for them. Give those hosts a certificate some other way, for example a static site block or a wildcard certificate.

### 4.4 How each request is routed

These checks run in order, before any page or API:

1. If the proxy sends `X-Forwarded-Proto: https`, the request is treated as HTTPS, so redirects are correct behind proxies such as Railway.
2. `/health` is always answered, for load balancers.
3. `/tls/ask` is answered as in 4.3.
4. **Platform host** (`admin.<base_domain>` or `platform_host`): `/` redirects to `/platform`. Only `/platform...` and `/api/platform/...` are served, and everything else is "page not found". On **any other host**, the platform paths are "page not found".
5. **Docs host** (`docs.<base_domain>` or `docs_host`): `/` redirects to `/docs`. Only `/docs...` and `/api/docs/...` are served, to logged-in docs viewers; everything else is "page not found". On any other host, the docs paths are "page not found".
6. **Bare base domain:** Only these are served: `/` (the landing page), `/api/v1/workspaces/lookup`, `/static/public/static/*`, `/oauth/*` (the shared OAuth redirect, which passes the browser on to the right workspace), `/webhook/meta` and `/meta/*` (Meta's shared webhook and callbacks). Anything else gets "Workspace not found".
7. **Any other host** is resolved to a workspace:
   - `<slug>.<base_domain>` is looked up by slug. Only direct subdomains count, so `a.b.<base_domain>` doesn't match.
   - Any other hostname is looked up as a verified custom domain that is the workspace's primary domain or an active help centre's domain.
   - Results are cached for 1 minute, and "not found" results for 10 seconds. Changes made in the dashboard clear the cache.
8. What the visitor sees for each outcome:

| Outcome | What the visitor sees |
|---|---|
| Not found | "Workspace not found" (404) |
| Scheduled for deletion | "This workspace is being deleted, with all its data, on <date>. Until then support can restore it." (403) |
| Suspended | "This workspace has been suspended. Contact support to restore access" (403) |
| Active | The request continues to the workspace's pages and APIs. |

9. With multi-tenancy **off**, every host is the single default workspace. There is no landing page, platform dashboard or custom-domain flow.

---

## 5. Key `config.sample.toml` sections (brief)

| Section and key | What it controls |
|---|---|
| `[app]` `log_level`, `env` | Logging detail. `env = "dev"` gives coloured logs. |
| `[app]` `encryption_key` | **Must be exactly 32 characters** (the server won't start otherwise). It encrypts secrets such as two-factor secrets and mail passwords. Generate one with `openssl rand -hex 16`. The server warns if you are still using the sample key. |
| `[app]` `check_updates`, `update_check_url` | An update banner in the admin panel. It only works when a feed URL is set. |
| `[app]` `static_dir` | The same as `--static-dir`. |
| `[app.legal]` `privacy_policy_url`, `dpa_url`, `subprocessors_url` | Links shown on every workspace's Admin → Privacy page. Empty links are hidden. |
| `[app.tenancy]` `enabled`, `base_domain` | Multi-tenancy on/off and the parent domain. `base_domain` is required when enabled. It can carry a port for local development, such as `localhost:9000`. |
| `url_scheme` | `https`, or `http` for local development. Used in links and emails. |
| `product_name` | The name shown on the landing page. |
| `default_plan` | The plan new workspaces start on. |
| `upgrade_url` | The upgrade button target on Admin → Usage. |
| `platform_host` | The dashboard host. Default `admin.<base_domain>`. Use a host no workspace can take. |
| `docs_host` | The documentation host. Default `docs.<base_domain>`, i.e. docs.desk.arrlink.com in production. |
| `platform_require_2fa` | Forces two-factor login for platform admins. Default true. |
| `domain_recheck_interval` | How often verified domains are re-checked. Default `24h`. |
| `deletion_grace_days` | Days a deleted workspace stays restorable. Default 30. |
| `export_max_files_mb` | The most stored files a workspace data export includes. Default 2048. |
| `queue_share_percent` | The most of any shared background queue one workspace may fill (webhooks, incoming mail, automation, AI replies, notification email). Default 10%. Work over that share is dropped. |
| `dns_server` | A `host:port` used for domain TXT lookups. Empty uses the system resolver. |
| `[app.tenancy.plans.<name>]` | Plan limits (section 4.1). |
| `[oauth.google]`, `[oauth.microsoft]` | The installation's own OAuth apps for one-click Gmail and Outlook inboxes. Redirect URL: `https://<base_domain>/oauth/<provider>/callback`. |
| `[meta]` | The shared Meta app for WhatsApp, Messenger and Instagram: `app_id`, `app_secret`, `graph_version`, `webhook_verify_token`, `whatsapp_config_id` (required for WhatsApp), `login_config_id`, and `human_agent` (7-day reply tag, only after App Review). The webhook is `https://<base_domain>/webhook/meta`. |
| `[app.server]` | Bind `address` (default `0.0.0.0:9000`), `socket`, `disable_secure_cookies` (don't use in production), `session_lifetime` (agent sessions, 9h), timeouts, `max_body_size` (100 MB), `read_buffer_size`, `keepalive_timeout`. |
| `[upload]`, `[upload.fs]`, `[upload.s3]` | File storage: `fs` (a directory, with signed-link expiry) or `s3` (endpoint, `presign_url` for a public address, keys, region, bucket, path, expiry). To move from `fs`, use `--copy-uploads-to-s3`. |
| `[db]` | PostgreSQL connection and pool sizes. |
| `[redis]` | The Redis connection, or a `url`. It holds two-factor challenges, rate limits and more. |
| `[message]`, `[notification]`, `[automation]`, `[ai_agent]`, `[autoassigner]`, `[webhook]`, `[integration]` | Worker counts, queue sizes and intervals. For the AI agent: `max_steps`, `followup_interval`, `hand_back_after`, `max_history_messages`. |
| `[slack]`, `[teams]` | The shared Slack app and Microsoft Teams bot credentials for those integrations. |
| `[ssrf]` | Blocks outbound requests to private or internal IP addresses for admin-entered URLs (webhooks, OIDC, AI). **Turn it on for hosted multi-workspace installations.** `allowed_cidrs` lists exceptions. |
| `[conversation]`, `[sla]` | Unsnooze interval, how long drafts are kept, continuity email scans, and SLA evaluation interval. |
| `[rate_limit.*]` | Per-IP, per-minute limits: `widget` (100), `auth` (30, which also covers the platform login, two-factor, test mail, impersonation link and landing lookup), `public` (100), `media` (300), and webhooks for Meta, Telegram, voice, Slack and Teams (1200 each). |
| `[voice]` | How often Exotel call lists are re-read (`reconcile_interval`, `"0"` turns it off) and how far back each check looks (`reconcile_lookback`). |

---

### Things worth knowing that the screens don't spell out

1. **Platform admins are managed only from the command line.** They are added, removed, given new passwords and have two-factor reset with `--platform-admin-*`. Docs viewers, by contrast, are managed in the dashboard under Docs access, or with `--docs-viewer-*`.
2. **No voluntary 2FA.** With `platform_require_2fa = false` there is no way in the dashboard to turn two-factor login on.
3. **The "Suspended" filter includes deletions.** It also lists workspaces scheduled for deletion; their badge says "Deleting".
4. **Re-enabling a channel doesn't revive inboxes.** The inboxes it switched off stay off until the workspace admin turns them on again.
5. **CLI activate vs restore.** `--tenant-activate` on a workspace scheduled for deletion does not cancel the deletion. Use `--tenant-restore`.
6. **CLI isn't audited, and CLI delete is instant.** `--tenant-*` actions aren't audited, and `--tenant-delete` skips the grace period.
7. **The default workspace is special.** It (ID 1, slug "default") can't be suspended or deleted, has no plan limits, and has no Danger zone in the dashboard.