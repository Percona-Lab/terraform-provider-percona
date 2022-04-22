package main

import (
	"context"
	"flag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"log"
	"terraform-percona/internal/provider"
)

func main() {
	var debugMode bool

	flag.BoolVar(&debugMode, "debug", true, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := &plugin.ServeOpts{ProviderFunc: provider.New}

	if debugMode {
		err := plugin.Debug(context.Background(), "terraform-percona.com/terraform-percona/percona", opts)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	plugin.Serve(opts)
}
