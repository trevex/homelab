export k3s_server_ip := "https://192.168.1.121:6443"

build-dir:
    mkdir -p .build

coreos-iso: build-dir
    #!/usr/bin/env bash
    set -euxo pipefail

    cd .build

    if [ -f fcos.iso ]; then
        echo "'fcos.iso' already exist, skipping download (if you want to update run 'just clean' to fetch newest image on next run"
    else
        docker run --pull=always --rm -v .:/data -w /data \
            quay.io/coreos/coreos-installer:release download -s stable -p metal -f iso --decompress | tee last-iso.txt
        mv "$(cat last-iso.txt)" "fcos.iso"
    fi

node-iso $hostname $disk $k3s_role: coreos-iso
    #!/usr/bin/env bash
    set -euxo pipefail

    cd .build

    export HOSTNAME=${hostname}
    export K3S_ROLE=${k3s_role}
    export K3S_TOKEN="changethistoanythingbutthis"
    export SSH_AUTHORIZED_KEY=$(ssh-agent sh -c 'ssh-add -q; ssh-add -L' | head -n 1)
    export K3S_EXTRA_FLAGS=""

    if [ $K3S_ROLE == "agent" ]; then
        export K3S_EXTRA_FLAGS="-server ${k3s_server_ip}"
    fi

    envsubst < ../butane.yaml > "${hostname}.yaml"

    butane --pretty --strict "${hostname}.yaml" --output "${hostname}.ign"

    docker run --rm -v .:/data -w /data \
        quay.io/coreos/coreos-installer:release iso customize \
        --dest-ignition "/data/${hostname}.ign" \
        --dest-device "${disk}" \
        -o "/data/${hostname}.iso" \
        /data/fcos.iso


controller-1: (node-iso "controller-1" "/dev/sda" "server")

compute-1: (node-iso "compute-1" "/dev/sda" "agent")

compute-2: (node-iso "compute-2" "/dev/sda" "agent")

[confirm]
dd hostname device: build-dir
    sudo dd if=.build/{{hostname}}.iso of={{device}} bs=1M status=progress

[confirm]
clean:
    rm -rf .build

kubeconfig:
    ssh core@$(echo "${k3s_server_ip}" | awk -F'https://|:[0-9]+' '$0=$2') sudo cat /etc/rancher/k3s/k3s.yaml > .build/kubeconfig.raw
    sed "s/https:\/\/127\.0\.0\.1:6443/$(echo $k3s_server_ip | sed 's/\//\\\//g')/g" .build/kubeconfig.raw > .build/kubeconfig

k *args:
    KUBECONFIG=`pwd`/.build/kubeconfig kubectl {{args}}

install: (k "apply -k bgp-evpn")

uninstall: (k "delete -k bgp-evpn")

vtysh $hostname:
    #!/usr/bin/env bash
    set -euxo pipefail

    export KUBECONFIG=`pwd`/.build/kubeconfig

    pod_name=$(kubectl  get pods --all-namespaces --template "{{{{range .items}}{{{{if eq .spec.nodeName \"${hostname}\"}}{{{{.metadata.name}}{{{{\"\n\"}}{{{{end}}{{{{end}}" | grep "route-reflector\|vtep")
    echo "Found $pod_name on $hostname."

    kubectl  -n kube-system exec -it "$pod_name" -- vtysh
