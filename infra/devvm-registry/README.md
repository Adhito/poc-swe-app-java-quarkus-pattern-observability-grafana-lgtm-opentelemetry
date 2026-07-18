# Dev VM registry (PRD D9 — adapted for Docker Engine)

Runs `docker.io/library/registry:2.8.3` under **Docker Engine** on the Dev VM,
listening on `192.168.56.20:5000` (plain HTTP — POC-internal network only).
Cluster nodes trust it via the Ansible playbook in [../ansible](../ansible).

> **Deviation from D9:** the PRD specified Podman + a Quadlet unit, but the
> Dev VM runs Docker Engine (29.x). The Quadlet existed only because rootless
> Podman ignores `--restart=always` at boot; Docker's root daemon honors it,
> so a plain `docker run --restart=always` is the whole persistence story.
> Jib builds are unaffected either way — Jib is daemonless and pushes over
> HTTP itself (`quarkus.container-image.insecure=true`).

## Install (on the Dev VM)

```bash
mkdir -p /home/vagrant/registry-data

docker run -d \
  --name registry \
  --restart=always \
  -p 5000:5000 \
  -v /home/vagrant/registry-data:/var/lib/registry \
  docker.io/library/registry:2.8.3
```

`--restart=always` + the dockerd systemd unit means it comes back on reboot —
verify once with `sudo reboot` then `docker ps` if you want certainty.

## One-time daemon config (only for manual docker push/pull tests)

The Docker CLI refuses plain-HTTP registries unless listed in
`/etc/docker/daemon.json`. **Jib does not need this** — it's only for the
round-trip test below and any manual `docker push/pull` against the registry.

```bash
sudo tee /etc/docker/daemon.json > /dev/null <<'EOF'
{
  "insecure-registries": ["192.168.56.20:5000"]
}
EOF
sudo systemctl restart docker   # registry container auto-restarts (restart=always)
```

(If `daemon.json` already has content, merge the key instead of overwriting.)

## Verify

```bash
# on the Dev VM
docker ps --filter name=registry                # -> Up ...
curl http://localhost:5000/v2/_catalog          # -> {"repositories":[]}

# from a cluster node (Phase 0 exit criterion, PRD §7 / D9)
curl http://192.168.56.20:5000/v2/_catalog      # -> JSON, proves reachability
```

## Push/pull round trip (Phase 0 exit criterion)

```bash
# Dev VM (needs the daemon.json step above)
docker pull docker.io/library/alpine:3.20
docker tag docker.io/library/alpine:3.20 192.168.56.20:5000/test:1
docker push 192.168.56.20:5000/test:1

# any cluster node (after the Ansible trust playbook has run)
sudo crictl pull 192.168.56.20:5000/test:1
```

## Notes

- Registry data lives in `/home/vagrant/registry-data` on the VM disk (→ external SSD per D9).
- Optional (backlog): a second instance configured as a Docker Hub pull-through cache.
