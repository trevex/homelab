# `homelab`

Currently 3 mini-PCs with N100 16GB RAM and 512GB SSD running K3s on FCOS.

## Prerequisites

An environment with `docker`, `direnv`, `nix` and `nix-direnv` available.
Make sure to update the `justfile`-recipes for your environment.

## Generate node-specific ISOs

Homelab does not support PXE boot and I wanted to keep it simple, so for every
node a new ISO is generated with the configuration embedded.

```bash
just controller-1 # equivalent to just node-iso "controller-1" "/dev/sda" "server"
```
_**NOTE**_: Behind the scenes `just node-iso` is used which takes a `hostname`, `device` (to install on) and K3s role (`"server" || "agent"`).

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

**After the controller is installed, make sure to update `k3s_server_ip` in [`justfile`](./justfile)!**

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

## Connect to the K3s

Retrieve and patch the kubeconfig from the controller:
```bash
just kubeconfig
```

You can then either invoke kubectl via:
```bash
just k get po --all-namespaces
```

Or alternatively simply export `KUBECONFIG` pointing to the config and using `kubectl` and related tool:
```bash
export KUBECONFIG=.build/kubeconfig
kubectl get po --all-namespaces
```

## Install cluster components

We want a route-reflector and a daemonset of VTEPs, as well as Multus and custom CNI components to be installed.
With the kubeconfig available, run:
```bash
just install
```
