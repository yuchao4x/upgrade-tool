/*
Copyright 2023 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in
compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is
distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing permissions and limitations under the
License.
*/

package start

import (
	"github.com/spf13/cobra"

	"github.com/jhernand/upgrade-tool/internal"
	"github.com/jhernand/upgrade-tool/internal/exit"
)

// StartBundleServer creates and returns the `start bundle-server` command.
func StartBundleServer() *cobra.Command {
	command := &startBundleServerCommand{}
	result := &cobra.Command{
		Use:   "bundle-server",
		Short: "Starts the HTTP server that serves the upgrade bundle",
		Args:  cobra.NoArgs,
		RunE:  command.run,
	}
	flags := result.Flags()
	flags.StringVar(
		&command.flags.root,
		"root",
		"",
		"Filesystem root. If this is specified then the rest of the paths will be "+
			"relative to it.",
	)
	flags.StringVar(
		&command.flags.bundleFile,
		"bundle-file",
		"",
		"Path of the bundle file previously copied or mounted to the node.",
	)
	flags.StringVar(
		&command.flags.listenAddr,
		"listen-addr",
		":8080",
		"Listen address",
	)
	return result
}

type startBundleServerCommand struct {
	flags struct {
		root       string
		listenAddr string
		bundleFile string
	}
}

func (c *startBundleServerCommand) run(cmd *cobra.Command, argv []string) error {
	// Get the context:
	ctx := cmd.Context()

	// Get the dependencies from the context:
	logger := internal.LoggerFromContext(ctx)

	// Check the flags:
	ok := true
	if c.flags.listenAddr == "" {
		logger.Error(nil, "Listen address is mandatory")
		ok = false
	}
	if c.flags.bundleFile == "" {
		logger.Error(nil, "Bundle file is mandatory")
		ok = false
	}
	if !ok {
		return exit.Error(1)
	}

	// Create and start the server:
	server, err := internal.NewBundleServer().
		SetLogger(logger).
		SetBundleFile(c.flags.bundleFile).
		SetListenAddr(c.flags.listenAddr).
		Build()
	if err != nil {
		logger.Error(err, "Failed to create server")
		return exit.Error(1)
	}
	err = server.Run(ctx)
	if err != nil {
		logger.Error(err, "Failed to run server")
		return exit.Error(1)
	}

	return nil
}
