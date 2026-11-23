package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/provider"
)

// Generate the Terraform Registry documentation in docs/ from the provider
// schema, examples/ and templates/.
//go:generate go tool tfplugindocs generate --provider-name artisanvirt

// version is set by GoReleaser at build time (-X main.version=...).
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		// The registry address users write in required_providers. Keep it in
		// sync with the GitHub repository the Terraform Registry reads from.
		Address: provider.RegistryAddress,
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
