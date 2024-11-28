# DNS-over-HTTPS

This is a fork of the original DNS-over-HTTPS project [https://github.com/m13253/dns-over-https](https://github.com/m13253/dns-over-https), with added support for caching DNS responses in Redis and highly secure container image.

Forked at version 2.3.3

## Environment

```bash
UPSTREAM_DNS_SERVER="udp:208.67.222.222:53"
DOH_HTTP_PREFIX="/getnsrecord"
DOH_SERVER_LISTEN_PORT="8053"
REDIS_URL="redis:6379"
DOH_SERVER_TIMEOUT="10"
DOH_SERVER_TRIES="3"
DOH_SERVER_VERBOSE="false"
```

## Build

```bash
podman build -t dns-over-https .
```

## Run

```bash
podman run --rm \
  -e UPSTREAM_DNS_SERVER="udp:208.67.222.222:53" \
  -e DOH_HTTP_PREFIX="/getnsrecord" \
  -e DOH_SERVER_LISTEN_PORT="8053" \
  -e REDIS_URL="redis:6379" \
  -p 8053:8053/tcp \
  -p 8053:8053/udp \
  dns-over-https
```

## Todo

- [x] Podman-Compose with Redis and DNS-over-HTTPS
- [ ] Kubernetes Deployment example with Redis and DNS-over-HTTPS
- [x] Integration Test
