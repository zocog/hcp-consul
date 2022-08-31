services {
  name = "s2"
  port = 8181
  address = "172.25.1.1"
  connect {
    sidecar_service {
      address = "172.25.1.0"
      proxy {
        local_service_address = "172.25.1.1"
      }
      checks {
        name = "Connect Sidecar Listening"
        tcp = "172.25.1.0:21001"
        interval = "10s"
      }
    }
  }
}