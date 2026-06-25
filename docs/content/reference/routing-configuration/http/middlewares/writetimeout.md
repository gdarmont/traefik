---
title: "Traefik WriteTimeout Documentation"
description: "In Traefik Proxy's HTTP middleware, WriteTimeout overrides the EntryPoint response write deadline for matching requests. Read the technical documentation."
---

The `writeTimeout` middleware overrides, for the requests it handles, the response write deadline otherwise enforced by the [EntryPoint `transport.respondingTimeouts.writeTimeout`](../../../install-configuration/entrypoints.md#opt-transport-respondingTimeouts-writeTimeout).

It is typically used to keep long-lived streaming responses, such as Server-Sent Events (SSE), alive on a single route without raising the write timeout for every other route served by the EntryPoint.

!!! info "This middleware only has an effect when the EntryPoint write timeout is set"

    The EntryPoint `transport.respondingTimeouts.writeTimeout` defaults to `0`, which means no write deadline is enforced at all.
    On such an EntryPoint there is nothing to override, and this middleware does nothing.

!!! tip

    A `timeout` of `0` disables the write deadline for the matching requests, which is the recommended value for endpoints streaming responses indefinitely (e.g. SSE).

!!! warning "Routes serving protocol upgrades"

    The deadline is absolute, not idle-based: it applies from the moment the middleware runs until the response is fully written.
    On a connection that is hijacked for a protocol upgrade (WebSocket, SPDY), the deadline is never cleared, so a positive `timeout`
    closes the tunnel that many seconds after the upgrade regardless of activity. Use `timeout: 0` on routes serving upgrades.

## Configuration Examples

```yaml tab="Structured (YAML)"
http:
  middlewares:
    test-writetimeout:
      writeTimeout:
        timeout: 0
```

```toml tab="Structured (TOML)"
[http.middlewares]
  [http.middlewares.test-writetimeout.writeTimeout]
    timeout = 0
```

```yaml tab="Labels"
labels:
  - "traefik.http.middlewares.test-writetimeout.writeTimeout.timeout=0"
```

```json tab="Tags"
{
  //...
  "Tags" : [
    "traefik.http.middlewares.test-writetimeout.writeTimeout.timeout=0"
  ]
}
```

```yaml tab="Kubernetes"
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: test-writetimeout
spec:
  writeTimeout:
    timeout: 0
```

## Configuration Options

| Field                        | Description         | Default | Required |
|:-----------------------------|:------------------------------------------|:--------|:---------|
| <a id="opt-timeout" href="#opt-timeout" title="#opt-timeout">`timeout`</a> | Maximum duration for writing the response to the client before the connection is closed. <br /> A value of `0` disables the write deadline for the matching requests. <br /> Negative values are rejected. | 0s | No |
