package consul

import "github.com/hashicorp/consul/logging"

func init() {
	RegisterEndpoint(func(s *Server) interface{} { return &ACL{s, s.loggers.Named(logging.ACL)} })
	RegisterEndpoint(func(s *Server) interface{} { return &Catalog{s, s.loggers.Named(logging.Catalog)} })
	RegisterEndpoint(func(s *Server) interface{} { return NewCoordinate(s, s.logger) })
	RegisterEndpoint(func(s *Server) interface{} { return &ConfigEntry{s, s.loggers.Named(logging.ConfigEntry)} })
	RegisterEndpoint(func(s *Server) interface{} { return &ConnectCA{srv: s, logger: s.loggers.Named(logging.Connect)} })
	RegisterEndpoint(func(s *Server) interface{} { return &FederationState{s} })
	RegisterEndpoint(func(s *Server) interface{} { return &DiscoveryChain{s} })
	RegisterEndpoint(func(s *Server) interface{} { return &Health{s, s.loggers.Named(logging.Health)} })
	RegisterEndpoint(func(s *Server) interface{} { return &Intention{s, s.loggers.Named(logging.Intentions)} })
	RegisterEndpoint(func(s *Server) interface{} { return &Internal{s, s.loggers.Named(logging.Internal)} })
	RegisterEndpoint(func(s *Server) interface{} { return &KVS{s, s.loggers.Named(logging.KV)} })
	RegisterEndpoint(func(s *Server) interface{} { return &Operator{s, s.loggers.Named(logging.Operator)} })
	RegisterEndpoint(func(s *Server) interface{} { return &PreparedQuery{s, s.loggers.Named(logging.PreparedQuery)} })
	RegisterEndpoint(func(s *Server) interface{} { return &Session{s, s.loggers.Named(logging.Session)} })
	RegisterEndpoint(func(s *Server) interface{} { return &Status{s} })
	RegisterEndpoint(func(s *Server) interface{} { return &Txn{s, s.loggers.Named(logging.Transaction)} })
}
