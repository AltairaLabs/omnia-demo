# personal-agent

The protocol side of "Ellen's assistant", a Personal Agent under the
[Personal Agent Protocol](https://personalagentprotocol.org/docs/1.0/spec)
(PAP 1.0). Ellen's assistant itself is an Omnia agent; it calls this
service's tools over HTTP. This service holds the agent's keys and Ellen's
User IDs, discovers Companies, and holds their conversations.

It is a **stand-in** for a consumer Personal Agent run by another company.
See `docs/stand-ins.md` and the comment at the top of `main.go` for what it
fakes and what that changes.

## Packages

| Package | What it does |
|---|---|
| `identity` | The client metadata document, `jwks.json`, delegations and Session JWTs (§4.1, §5.2). |
| `discovery` | A Company's `poppy.json` and its issuer (§3). |
| `conversation` | Start, send, read, close and Direct Conversations; a Reader per conversation; the Store (memory or Redis) (§8). |

## HTTP surface

| Route | Who calls it | What it does |
|---|---|---|
| `GET /agent.json`, `GET /jwks.json` | Companies' poppy facades (public) | The client metadata and public keys, served directly with no redirect. |
| `GET /poppy-test/{file}` | The PAP validator (Task 13 only) | Files from `--test-identity-dir`, when it is set. |
| `POST /tools/find_company` | The assistant | `{"domain"}` → `{"name","domain","agents":[{"description","access"}]}`, or `{"found":false,"reason":"no Poppy agent at <domain>"}` with HTTP 200. |
| `POST /tools/start_conversation` | The assistant | `{"domain","message"}` → `{"conversation_id","company"}`. The domain must be in `--known-companies` or found with `find_company`. Starts a Reader. |
| `POST /tools/send_message` | The assistant | `{"conversation_id","text"}`. |
| `POST /tools/read_conversation` | The assistant | `{"conversation_id"}` → `{"company","working","handoff","messages":[{"role","author","text"}],"closed"}`, plus `"stopped"` when the Reader gave up (e.g. a Company asking for sign-in). Read from the Store only. |
| `POST /tools/close_conversation` | The assistant | `{"conversation_id"}`. |
| `POST /api/direct` | The page | `{"conversation_id","text"}`: Ellen takes a conversation over. Her first line opens a Direct Conversation (§8.10); later lines go to the same one. |
| `GET /api/conversations` | The page | Every stored conversation, oldest first: `[{"conversation_id","company","parent_conversation_id","working","handoff","closed","stopped","messages":[{"role","author","text"}]}]`, from the Store alone. A page loads it first, then follows `/api/updates`. |
| `GET /api/updates` | The page | Server-sent events, one `{"conversation_id","company","event"}` line per new event (`parent_conversation_id` too on a Direct Conversation's). |
| `GET /healthz` | Kubernetes | 200. |

Tool failures answer `{"error": "..."}` with a 4xx or 502. No answer carries a
User ID, a key or a Session JWT. The assistant may not send or close in a
Direct Conversation: that is Ellen's own.

Only `/agent.json`, `/jwks.json` (and `/poppy-test/` during Task 13) are
public. The tools are reached in-cluster, and `/api/*` through the page, which
sits behind the demo catalogue's sign-in.

## Flags

| Flag | Default | |
|---|---|---|
| `--identity-dir` | `/var/run/personal-agent/identity` | `es256.pem` and `ps384.pem`, PKCS#8 (Secret `personal-agent-identity`). |
| `--base-url` | `https://assistant.azure.altairalabs.ai` | The origin of `client_id` and `jwks_uri`. |
| `--redis-url` | empty | Store state in Redis. Empty keeps it in memory, lost on restart. |
| `--addr` | `:8080` | |
| `--test-identity-dir` | empty | Serve `/poppy-test/` from here. |
| `--known-companies` | empty | Comma-separated domains Ellen has accounts at. |

On startup it restarts a Reader for every stored conversation that is neither
closed nor stopped.

## Build and test

```bash
go test -race ./demos/personal-agent/...
docker build -f demos/personal-agent/Dockerfile .   # from the repo root
```

`PA_TEST_REDIS_URL=redis://localhost:6379/15` also runs the Redis store test
(it flushes that database).
