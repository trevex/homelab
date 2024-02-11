variable "controllers" {
  type = map(object({
    mac  = string
  }))
}

variable "ssh_authorized_keys" {
  type = set(string)
}

provider "matchbox" {
  endpoint    = "127.0.0.1:8081"
  client_cert = file("${path.module}/.matchbox/tls/client.crt")
  client_key  = file("${path.module}/.matchbox/tls/client.key")
  ca          = file("${path.module}/.matchbox/tls/ca.crt")
}

provider "ct" {}

terraform {
  required_providers {
    ct = {
      source  = "poseidon/ct"
      version = "0.13.0"
    }
    matchbox = {
      source = "poseidon/matchbox"
      version = "0.5.2"
    }
  }
}

data "ct_config" "controllers" {
  for_each = var.controllers

  content = templatefile("${path.module}/butane/controller.yaml", {
    ssh_authorized_keys = var.ssh_authorized_keys
  })
  strict   = true
}

resource "matchbox_profile" "controllers" {
  for_each = var.controllers

  name  = each.key

  kernel = "not-required-installing-from-usb"
  initrd = ["not-required-installing-from-usb"]
  args   = ["not-required-installing-from-usb"]

  raw_ignition = data.ct_config.controllers[each.key].rendered
}

resource "matchbox_group" "controllers" {
  for_each = var.controllers

  name  = each.key
  profile = matchbox_profile.controllers[each.key].name

  selector = {
    mac = each.value.mac
  }
}


