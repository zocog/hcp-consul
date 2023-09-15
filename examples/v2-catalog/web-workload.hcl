ID {
  Type = gvk("catalog.v1alpha1.Workload")
  Name = "web-123abc"
  Tenancy = {
    Partition = "default"
    Namespace = "default"
    PeerName = "local"
  }
}

Data {
  Addresses = [
    // This will the be the pod IP in Kube.
    { Host = "127.0.0.1" },
  ]
  Ports "tcp" {
    Port     = 19090
    Protocol = "PROTOCOL_TCP"
  }

  #  Ports "grpc" {
  #    Port     = 9090
  #    Protocol = "PROTOCOL_GRPC"
  #  }

  Ports "mesh" {
    Port     = 20001
    Protocol = "PROTOCOL_MESH"
  }

  Identity = "web"
}
