# Dev VM registry (PRD D9)

Runs `docker.io/library/registry:2.8.3` under rootless Podman on the Dev VM,
listening on `192.168.56.20:5000` (plain HTTP — POC-internal network only).
Cluster nodes trust it via the Ansible playbook in [../ansible](../ansible).

## Install (on the Dev VM, as the `vagrant` user)

```bash
mkdir -p /home/vagrant/registry-data
mkdir -p ~/.config/containers/systemd
cp registry.container ~/.config/containers/systemd/

# make the user unit start at boot, without an active login session
sudo loginctl enable-linger vagrant

systemctl --user daemon-reload
systemctl --user start registry.service
```

## Verify

```bash
# on the Dev VM
systemctl --user status registry.service
curl http://localhost:5000/v2/_catalog          # -> {"repositories":[]}

# from a cluster node (Phase 0 exit criterion, PRD §7 / D9)
curl http://192.168.56.20:5000/v2/_catalog      # -> JSON, proves reachability
```

## Push/pull round trip (Phase 0 exit criterion)

```bash
# Dev VM
podman pull docker.io/library/alpine:3.20
podman tag docker.io/library/alpine:3.20 192.168.56.20:5000/test:1
podman push --tls-verify=false 192.168.56.20:5000/test:1

# any cluster node (after the Ansible trust playbook has run)
sudo crictl pull 192.168.56.20:5000/test:1
```

## Notes

- Registry data lives in `/home/vagrant/registry-data` on the VM disk (→ external SSD per D9).
- Optional (backlog): a second instance configured as a Docker Hub pull-through cache.
