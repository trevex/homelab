module github.com/trevex/homelab/vxlan-cni

go 1.22

toolchain go1.22.1

require (
	github.com/containernetworking/cni v1.1.2
	github.com/containernetworking/plugins v1.4.0
	github.com/vishvananda/netlink v1.2.1-beta.2
	golang.org/x/sys v0.18.0
)

require (
	github.com/cybozu-go/netutil v1.4.7 // indirect
	github.com/onsi/gomega v1.31.1 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	golang.org/x/net v0.22.0 // indirect
)
