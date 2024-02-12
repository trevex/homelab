export k3s_server_ip := "https://192.168.1.121:6443"

build-dir:
    mkdir -p .build

coreos-iso: build-dir
    #!/usr/bin/env bash
    set -euxo pipefail

    cd .build

    if [ -f fcos.iso ]; then
        echo "'fcos.iso' already exist, skipping download (if you want to update run 'just clean' to fetch newest image on next run"
        exit -1
    fi

    docker run --pull=always --rm -v .:/data -w /data \
        quay.io/coreos/coreos-installer:release download -s stable -p metal -f iso --decompress | tee last-iso.txt
    mv "$(cat last-iso.txt)" "fcos.iso"


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

    docker run --pull=always --rm -v .:/data -w /data \
        quay.io/coreos/coreos-installer:release iso customize \
        --dest-ignition "/data/${hostname}.ign" \
        --dest-device "${disk}" \
        -o "/data/${hostname}.iso" \
        /data/fcos.iso


controller-1: (node-iso "controller-1" "/dev/sda" "server")

kubeconfig:
    ssh core@$(echo "${k3s_server_ip}" | awk -F'https://|:[0-9]+' '$0=$2') echo "hi"

[confirm]
dd hostname device: build-dir
    sudo dd if=.build/{{hostname}}.iso of={{device}} bs=1M status=progress

[confirm]
clean:
    rm -rf .build

