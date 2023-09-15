ID {
  Type = gvk("catalog.v1alpha1.Service")
  Name = "api"
  Tenancy = {
    Partition = "default"
    Namespace = "default"
    PeerName = "local"
  }
}

Data {
  Workloads = {
    Prefixes = ["api"]
  }
  Ports = [
    {
      TargetPort = "tcp"
      Protocol   = "PROTOCOL_TCP"
    },
    #    {
    #      VirtualPort = 90
    #      TargetPort  = "grpc"
    #      Protocol    = "PROTOCOL_GRPC"
    #    },
    {
      TargetPort = "mesh"
      Protocol   = "PROTOCOL_MESH"
    }
  ]
}
