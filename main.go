package main

import (
	"os"

	"github.com/hashicorp/consul/agent"
	"github.com/hashicorp/consul/command"
	"github.com/hashicorp/consul/lib"
	_ "github.com/hashicorp/consul/service_os"
)

func init() {
	lib.SeedMathRand()
}

func main() {
	deps := &agent.InjectedDependencies{
		EnterpriseMetaHelper: &agent.EnterpriseMetaHelperOSS{},
	}
	os.Exit(command.BuildCLI(command.NewConsulCommandFactory(deps)))
}
