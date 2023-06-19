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
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jhernand/upgrade-tool/internal"
	"github.com/jhernand/upgrade-tool/internal/exit"
)

// StartBundleCleaner creates and returns the `start bundle-cleaner` command.
func StartBundleCleaner() *cobra.Command {
	command := &startBundleLoaderCommand{}
	result := &cobra.Command{
		Use:   "bundle-cleaner",
		Short: "Starts the program that cleans after the upgrade",
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
		&command.flags.node,
		"node",
		"",
		"Name of the node where this is running.",
	)
	flags.StringVar(
		&command.flags.bundleDir,
		"bundle-dir",
		"/var/lib/upgrade",
		"Bundle directory.",
	)
	return result
}

type startBundleCleanerCommand struct {
	flags struct {
		root      string
		node      string
		bundleDir string
	}
}

func (c *startBundleCleanerCommand) run(cmd *cobra.Command, argv []string) error {
	// Get the context:
	ctx := cmd.Context()

	// Get the dependencies from the context:
	logger := internal.LoggerFromContext(ctx)

	// Check the flags:
	ok := true
	if c.flags.node == "" {
		logger.Error(nil, "Node is madatory")
		ok = false
	}
	if c.flags.bundleDir == "" {
		logger.Error(nil, "Bundle directory is mandatory")
		ok = false
	}
	if !ok {
		return exit.Error(1)
	}

	// Create the API client:
	scheme := runtime.NewScheme()
	core.AddToScheme(scheme)
	config, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "Failed to load API configuration")
		return exit.Error(1)
	}
	options := clnt.Options{
		Scheme: scheme,
	}
	client, err := clnt.New(config, options)
	if err != nil {
		logger.Error(err, "Failed to create API client")
		return exit.Error(1)
	}

	// Start and execute the bundle loader:
	loader, err := internal.NewBundleExtractor().
		SetLogger(logger).
		SetClient(client).
		SetNode(c.flags.node).
		SetBundleDir(c.flags.bundleDir).
		Build()
	if err != nil {
		logger.Error(err, "Failed to create loader")
		return exit.Error(1)
	}
	err = loader.Run(ctx)
	if err != nil {
		logger.Error(err, "Failed to execute loader")
		return exit.Error(1)
	}

	return nil
}
