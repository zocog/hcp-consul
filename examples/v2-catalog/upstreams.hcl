ID {
  Type = gvk("mesh.v1alpha1.Upstreams")
  Name = "web-upstreams"
  Tenancy = {
    Partition = "default"
    Namespace = "default"
    PeerName = "local"
  }
}

Data {
  Workloads = {
    Prefixes = ["web"]
  }

  Upstreams = [
    {
      DestinationRef = {
        Type    = gvk("catalog.v1alpha1.Service")
        Name    = "api"
        Tenancy = {
          Namespace = "default"
          Partition = "default"
          PeerName = "local"
        }
      }
      DestinationPort = "tcp"

      IpPort = {
        Ip   = "127.0.0.1"
        Port = 1234
      }
    }
  ]
}
