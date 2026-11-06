# Blast radius of the teleport-operator identity

This note records what a leaked teleport-operator identity could do, and what is left after the hardening in this change.

## The identity

Each management cluster (MC) runs one teleport-operator. It authenticates to Teleport as a per-MC Bot named after the MC (user `bot-<mc>`). A tbot sidecar joins with the `kubernetes` join method (ServiceAccount `giantswarm:teleport-operator`, static JWKS) and writes the identity to the Secret `identity-output` in namespace `giantswarm`. The operator re-reads that Secret every 20 minutes. The default identity TTL is about one hour.

The `teleport-operator` role in teleport-fleet (`kubernetes/shared/templates/role_teleport_operator.yaml`) used to grant the `token` resource with the verbs `list`, `create`, `read`, `update` and `delete`.

## Why token creation is administrative

Teleport's documentation treats any role that can manage join tokens as an administrative one; see the [join methods reference](https://goteleport.com/docs/reference/join-methods/). In Teleport v18, `ServerWithRoles.CreateToken` and `UpsertToken` check only the token verbs and admin-action MFA, which does not apply to bots. `where` conditions are not evaluated for tokens, so a label-scoped role cannot limit which tokens a caller may touch.

## What the old role allowed

All of the following are scoped to one Teleport cluster (production `teleport.giantswarm.io` or test `test.teleport.giantswarm.io`), not to one MC:

1. Becoming any unscoped bot: a join token with the `Bot` role that names an existing bot yields that bot's certificates and roles.
2. Reading every join secret: `list` returns every token in the cluster, and for classic tokens the token name is the secret.
3. Registering rogue agents: Node, Kube and App tokens can be created under any name.

The blast radius of one leaked identity is therefore a whole Teleport cluster. The identity lives on the MC, not on workload clusters.

## After this change

With this PR plus the matching teleport-fleet and prometheus-rules changes:

- The role is reduced to `create`, `read` and `delete`; nothing can list tokens.
- Every bot gets a per-MC, least-privilege role.
- Alerts page Shield when a bot creates a token whose roles are outside Node, Kube and App, or joins with the secret `token` method.

`read` is kept on purpose. Operator tokens are UUIDv4 names, so the name is the secret, and `read` is needed to detect server-side revocation. The operator looks tokens up by name with `GetToken` rather than listing them.

## Residual risk

- An attacker holding the identity can still create a `Bot`-role token, so every remaining bot must be per-MC and least-privilege.
- Classic join tokens do not bind resource names: a Kube token can register any kube cluster name.
- Only Teleport scopes (not production-ready in 18.7, behind `TELEPORT_UNSTABLE_SCOPES`) or one leaf cluster per customer would remove these.
- Tokens no longer referenced by a cluster's Secret or ConfigMap (replaced during rotation, or whose object was deleted by hand) are no longer swept; they remain until their 720h expiry. This is inherent in dropping `list`.
