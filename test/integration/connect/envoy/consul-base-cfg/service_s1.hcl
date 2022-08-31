services {
  name = "s1"
  port = 8080
  address = "172.25.1.3"
  connect {
    sidecar_service {
      address = "172.25.1.2"
      proxy {
        local_service_address = "172.25.1.3"
        upstreams = [
          {
            local_bind_address = "0.0.0.0"
            destination_name = "s2"
            local_bind_port = 5000
          }
        ]
      }
      checks {
        name = "Connect Sidecar Listening"
        tcp = "172.25.1.2:21000"
        interval = "10s"
      }
    }
  }
}