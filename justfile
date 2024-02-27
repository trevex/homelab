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

    envsubst < ../fcos-k3s/butane.yaml > "${hostname}.yaml"

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
    @KUBECONFIG=`pwd`/.build/kubeconfig kubectl {{args}}

install: (k "apply -k bgp-evpn")

uninstall: (k "delete -k bgp-evpn")

vtysh hostname *args:
    #!/usr/bin/env bash
    set -euxo pipefail

    export KUBECONFIG=`pwd`/.build/kubeconfig

    pod_name=$(kubectl get pods --all-namespaces --template "{{{{range .items}}{{{{if eq .spec.nodeName \"{{hostname}}\"}}{{{{.metadata.name}}{{{{\"\n\"}}{{{{end}}{{{{end}}" | grep "route-reflector\|vtep")
    echo "Found ${pod_name} on {{hostname}}."

    kubectl  -n kube-system exec -it "${pod_name}" -- vtysh {{args}}

get-ip hostname:
    @just k get no {{hostname}} -o wide | tail -n1 | awk '{print $6}'

ssh hostname *args:
    #!/usr/bin/env bash
    set -euxo pipefail

    export KUBECONFIG=`pwd`/.build/kubeconfig

    node_ip=$(just get-ip {{hostname}})
    echo "Found IP ${node_ip} for node {{hostname}}."

    ssh core@"${node_ip}" {{args}}

vxlan-test-setup-node hostname dev cidr:
    #!/usr/bin/env bash
    set -euxo pipefail

    just ssh {{hostname}} <<EOF
    sudo ip link add vxlan200 type vxlan \
        id 200 \
        dstport 4789 \
        local $(just get-ip {{hostname}}) \
        nolearning
    sudo ip link add br200 type bridge
    sudo ip link set vxlan200 master br200
    sudo ip link set br200 type bridge stp_state 0
    sudo ip link set up dev br200
    sudo ip link set up dev vxlan200

    sudo ip link add dev {{dev}}s type veth peer name {{dev}}
    sudo ip link set dev {{dev}}s up
    sudo ip addr add {{cidr}} dev {{dev}}
    sudo ip link set dev {{dev}} up
    sudo ip link set {{dev}}s master br200
    EOF

vxlan-test-setup: (vxlan-test-setup-node "compute-1" "vm1" "172.16.16.2/24") (vxlan-test-setup-node "compute-2" "vm2" "172.16.16.3/24")

vxlan-test:
    #!/usr/bin/env bash
    set -euxo pipefail

    echo "Ping vm1 (compute-1) to vm2 (compute-2):"
    just ssh compute-1 <<EOF
    ping -I vm1 -c3 172.16.16.3
    EOF

    echo "Ping vm2 (compute-2) to vm1 (compute-1):"
    just ssh compute-2 <<EOF
    ping -I vm2 -c3 172.16.16.2
    EOF

vxlan-test-clean-node hostname dev:
    #!/usr/bin/env bash
    set -euxo pipefail

    just ssh {{hostname}} <<EOF
    sudo ip link set dev {{dev}} down
    sudo ip link set dev {{dev}}s down
    sudo ip link del dev {{dev}}s type veth peer name {{dev}}
    sudo ip link set down dev vxlan200
    sudo ip link set down dev br200
    sudo ip link del br200 type bridge
    sudo ip link del vxlan200 type vxlan
    EOF

vxlan-test-clean: (vxlan-test-clean-node "compute-1" "vm1") (vxlan-test-clean-node "compute-2" "vm2")
