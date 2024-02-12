# `homelab`

Currently 3 mini-PCs with N100 16GB RAM and 512GB SSD running K3s on FCOS.

## Prerequisites

An environment with `docker`, `direnv`, `nix` and `nix-direnv` available.

## Flash FCOS live DVD to USB stick

Homelab does not use PXE boot, so let's download FCOS directly and install from USB.

```bash
docker run --pull=always --rm -v .:/data -w /data \
    quay.io/coreos/coreos-installer:release download -s stable -p metal -f iso
```

Let's find the USB-stick and unmount if mounted:
```bash
df # or lsblk or dmesg
sudo umount /run/media/...
```

Let copy the data to the USB-stick:
```bash
sudo dd if=fedora-coreos-XXXXXXXXXX-live.x86_64.iso of=/dev/sdX bs=1M status=progress
```

## Setup and run matchbox

Before running matchbox with `docker` we need valid TLS certificates:
```bash
just gen-certs
```
Next start matchbox's server with:
```bash
just matchbox
```

You can test whether the certificates work as follows (but the following steps using `tofu` would throw errors as well):
```bash
openssl s_client -connect 127.0.0.1:8081 \
  -CAfile .matchbox/tls/ca.crt \
  -cert .matchbox/tls/client.crt \
  -key .matchbox/tls/client.key
```

## Deploy the profiles with `terraform`/`opentofu`

Before you deploy make sure [homelab.auto.tfvars](./homelab.auto.tfvars) contains the correct values.
The setup is kept as simple as possible and for now does not support HA control-plane as this would introduce complexity that is not required for the planed experiments.

Use `tofu` to generate the ignition configs and deploy them to matchbox:
```bash
tofu init
tofu apply
```

## Install FCOS

Change the boot order of the mini-PCs to boot from USB first and make sure neither secure boot nor fastboot are enabled.
(We will install to SSD but reinstall is required for config changes.)

Make sure matchbox is running and profiles were deployed.

Start with the controller-node before continuing to the worker.

Once the Live-ISO booted, run (replace variables):
```bash
sudo coreos-installer install /dev/sda --insecure-ignition \
    --ignition-url http://${MATCHBOX_IP_AND_PORT}/ignition?name=${PROFILE_NAME}
```

_NOTE_: We are not using mac and uuid as we are not PXE booting, but re-applying config by inserting USB.

## Testing FCOS

You should be able to SSH now:
```bash
ssh core@${NODE_IP}
```

On the node you can change to the root and test `kubectl`:
```bash
sudo -i
KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get po --all-namespaces
```
