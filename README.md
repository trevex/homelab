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

You can test whether the certificates work as follows:
```bash
openssl s_client -connect 127.0.0.1:8081 \
  -CAfile .matchbox/tls/ca.crt \
  -cert .matchbox/tls/client.crt \
  -key .matchbox/tls/client.key
```

## Deploy the profiles with `terraform`/`opentofu`

Before you deploy make sure [homelab.auto.tfvars](./homelab.auto.tfvars) contains the correct values.

Use `tofu` to generate the ignition configs and deploy them to matchbox:
```bash
tofu init
tofu apply
```
