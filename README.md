# ConectDesk

Modern, open source, self-hosted omnichannel customer support desk. Live chat, email, and more in a single binary.

Run it for one team, or as a multi-tenant service where every organisation signs up for its own workspace. See [MULTI_TENANCY.md](./MULTI_TENANCY.md).

## Features

- **Omnichannel inbox**  
  Live chat and email in one inbox. Every conversation lands in the same place, whichever channel it came from.
- **Live chat widget**  
  Embed a real-time chat widget on your website. Replies go out from the same inbox your team already works in.
- **AI assistant**  
  Answer live chat conversations automatically with an AI assistant grounded in your knowledge base. Hands off to a human when it can't help.
- **Agent copilot**  
  Draft replies, summarize conversations, and look up answers from the knowledge base without leaving the inbox.
- **Automations**  
  Rules that run on conversation events. Tag, assign, and route conversations based on conditions you define.
- **Granular permissions**  
  Role-based access control. Create custom roles with per-action permissions for teams and individual agents.
- **CSAT & analytics**  
  Send CSAT surveys automatically after a conversation closes. Track response times, resolution rates, and agent activity.
- **Custom attributes**  
  Create custom attributes for contacts or conversations such as the subscription plan or the date of their first purchase.
- **Macros**  
  Save replies you send often. One macro can send the message, set tags, and assign the conversation to a team.
- **Organization**  
  Tags, custom statuses, and snoozing to keep the inbox in order. Search covers every conversation.
- **Auto assignment**  
  Assign incoming conversations automatically, based on agent capacity or on criteria you define.
- **SLA management**  
  Set and track response time targets. Get notified when conversations are at risk of breaching SLA commitments.
- **SSO logins**  
  Google, Microsoft, and any OIDC provider are supported out of the box.
- **API**  
  HTTP/JSON APIs and webhooks for custom integrations and workflows.
- **Activity logs**  
  Track all actions performed by agents and admins, for auditing and accountability.
- **Command bar**  
  Opens with a simple shortcut (CTRL+K) and lets you quickly perform actions on conversations.


## Installation

Postgres needs the [pgvector](https://github.com/pgvector/pgvector) extension, which stores and searches the AI knowledge base. The `pgvector/pgvector:pg17` image ships with it; on a managed Postgres, enable `vector` before installing or upgrading.

### Railway

`railway.toml` builds the `Dockerfile`. Add Postgres (with pgvector) and Redis to the project, set the `CONECTDESK_*` variables, then run `railway up`.

__________________

### Docker

```shell
# Copy the config.sample.toml to config.toml and edit it as needed.
cp config.sample.toml config.toml

# Run the services in the background.
docker compose up -d

# Setting System user password.
docker exec -it conectdesk_app ./conectdesk --set-system-user-password
```

Go to `http://localhost:9000` and login with username `System` and the password you set using the `--set-system-user-password` command.


__________________

### Binary
- Build the binary with `make build`.
- Edit config.toml as needed.
- `./conectdesk --install` to setup the Postgres DB.
- Run `./conectdesk --set-system-user-password` to set the password for the System user.
- Run `./conectdesk` and visit `http://localhost:9000` and login with email `System` and the password you set using the --set-system-user-password command.

__________________

## Developers

- If you are interested in contributing, **please read [CONTRIBUTING.md](./CONTRIBUTING.md) first**.
- For local development and setup, see [CODEBASE_GUIDE.md](./CODEBASE_GUIDE.md).
- For planned features and project direction, see [ROADMAP.md](./ROADMAP.md).

The backend is written in Go and the frontend is Vue.js 3 with Shadcn UI.

## License

ConectDesk is based on [Libredesk](https://github.com/abhinavxd/libredesk) and is licensed under the [GNU AGPL v3](./LICENSE). See [NOTICE](./NOTICE).
