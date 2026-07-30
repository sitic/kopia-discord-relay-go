# kopia-discord-relay

A tiny relay that makes [Kopia](https://kopia.io) webhook notifications work
with Discord.

Kopia's webhook sender POSTs the notification as raw
`text/plain`/`text/html`, however Discord webhooks require a JSON payload.
This relay accepts Kopia's POST, wraps it in the JSON Discord expects
(splitting messages over Discord's 2000-character limit), and forwards it.

## Run

```bash
cp env.example .env   # optionally set BEARER_TOKEN
docker compose up -d --build
```

| Env var        | Required | Description                                       |
| -------------- | -------- | ------------------------------------------------- |
| `BEARER_TOKEN` | no       | If set, requests need `Authorization: Bearer <t>` |
| `LISTEN_ADDR`  | no       | Listen address, default `:8199`                   |

Generate a token with `openssl rand -hex 32`. If you expose the port beyond a
trusted network, set one.

## Point Kopia at it

Take your Discord webhook URL
(`https://discord.com/api/webhooks/<id>/<token>`) and replace the host with
the relay — the path carries the target, so the relay needs no per-channel
config and different machines/profiles can use different Discord channels:

```bash
kopia notification profile configure webhook \
  --profile-name=discord \
  --endpoint=http://yourserver:8199/api/webhooks/<id>/<token> \
  --method=POST --format=txt \
  --http-header="Authorization: Bearer YOUR_TOKEN" \
  --send-test-notification
```

The relay only forwards to `https://discord.com` and only to paths matching
Discord's webhook shape (`/api/webhooks/<id>/<token>`).

## Endpoints

- `POST /api/webhooks/<id>/<token>` — forwards to the same path on discord.com
- `GET /healthz` — health check, returns `200 ok`

Responses: `200` forwarded, `401` bad/missing token, `404` unknown path,
`502` Discord unreachable (shows up in Kopia's log; never fails the backup
itself).
