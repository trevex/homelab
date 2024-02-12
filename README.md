# `homelab`

Currently 3 mini-PCs with N100 16GB RAM and 512GB SSD running K3s on FCOS.

## Prerequisites

An environment with `docker`, `direnv`, `nix` and `nix-direnv` available.

## Generate node-specific ISOs

Homelab does not support PXE boot and I wanted to keep it simple, so for every
node a new ISO is generated with the configuration embedded.

```bash
just controller-1 # equivalent to just node-iso "controller-1" "/dev/sda" "server"
```
_*NOTE*_: Behind the scenes `just node-iso` is used which takes a hostname, install device and K3s role (`"server" || "agent"`).

Let's find the USB-stick and unmount if mounted:
```bash
df # or lsblk or dmesg
sudo umount /run/media/...
```

Let copy the data to the USB-stick:
```bash
just dd controller-1 /dev/sdX
```

## Install FCOS

Change the boot order of the mini-PCs to boot from USB first and make sure neither secure boot nor fastboot are enabled.
(We will install to SSD but reinstall is required for config changes.)

Start with the controller-node before continuing to the worker.

CoreOS will be installed with the embedded config automatically. Make sure to remove the USB after install to boot into the new installation.

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
