ip := ```
ip -4 addr show $(ip -br l | awk '$1 !~ "lo|vir|wl|docker" { print $1}') | grep -oP '(?<=inet\s)\d+(\.\d+){3}'
```

get-ip:
    @echo {{ip}}

# required by script
export SAN := "DNS.1:localhost,IP.1:127.0.0.1,IP.2:" + ip

gen-certs:
    #!/usr/bin/env bash
    set -euxo pipefail

    mkdir -p .matchbox/tls/
    cp openssl.conf .matchbox/tls/
    cd .matchbox/tls/

    echo $SAN

    # Copied from https://github.com/poseidon/matchbox/blob/main/scripts/tls/cert-gen
    # Licensed under Apache-2.0

    rm -f ca.key ca.crt server.key server.csr server.crt client.key client.csr client.crt index.* serial*
    rm -rf certs crl newcerts

    echo "Creating example CA, server cert/key, and client cert/key..."

    # basic files/directories
    mkdir -p {certs,crl,newcerts}
    touch index.txt
    touch index.txt.attr
    echo 1000 > serial

    # CA private key (unencrypted)
    openssl genrsa -out ca.key 4096
    # Certificate Authority (self-signed certificate)
    openssl req -config openssl.conf -new -x509 -days 3650 -sha256 -key ca.key -extensions v3_ca -out ca.crt -subj "/CN=fake-ca"

    # End-entity certificates

    # Server private key (unencrypted)
    openssl genrsa -out server.key 2048
    # Server certificate signing request (CSR)
    openssl req -config openssl.conf -new -sha256 -key server.key -out server.csr -subj "/CN=fake-server"
    # Certificate Authority signs CSR to grant a certificate
    openssl ca -batch -config openssl.conf -extensions server_cert -days 365 -notext -md sha256 -in server.csr -out server.crt -cert ca.crt -keyfile ca.key

    # Client private key (unencrypted)
    openssl genrsa -out client.key 2048
    # Signed client certificate signing request (CSR)
    openssl req -config openssl.conf -new -sha256 -key client.key -out client.csr -subj "/CN=fake-client"
    # Certificate Authority signs CSR to grant a certificate
    openssl ca -batch -config openssl.conf -extensions usr_cert -days 365 -notext -md sha256 -in client.csr -out client.crt -cert ca.crt -keyfile ca.key

    # Remove CSR's
    rm *.csr

    echo "*******************************************************************"
    echo "WARNING: Generated credentials are self-signed. Prefer your"
    echo "organization's PKI for production deployments."


matchbox_tls := `pwd` / ".matchbox/tls"
matchbox_data := `pwd` / ".matchbox/lib"

matchbox:
    mkdir -p {{matchbox_data}}/{profiles,groups,ignition,cloud,generic,assets}
    docker run -p 8080:8080 -p 8081:8081 --rm -v {{matchbox_data}}:/var/lib/matchbox:Z -v {{matchbox_tls}}:/etc/matchbox:Z quay.io/poseidon/matchbox:latest -address=0.0.0.0:8080 -rpc-address=0.0.0.0:8081 -log-level=debug


