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

package create

import (
	"github.com/spf13/cobra"

	"github.com/jhernand/upgrade-tool/internal"
	"github.com/jhernand/upgrade-tool/internal/exit"
)

// CreateBundle creates and returns the `create bundle` command.
func CreateBundle() *cobra.Command {
	command := &createBundleCommand{}
	result := &cobra.Command{
		Use:   "bundle",
		Short: "Creates upgrade bundle",
		Args:  cobra.NoArgs,
		RunE:  command.run,
	}
	flags := result.Flags()
	flags.StringVar(
		&command.flags.version,
		"version",
		"",
		"Version number, for example 4.13.4",
	)
	flags.StringVar(
		&command.flags.arch,
		"arch",
		"",
		"Architecture, for example x86_64",
	)
	flags.StringVar(
		&command.flags.outputDir,
		"output",
		"",
		"Output bundle directory",
	)
	flags.StringVar(
		&command.flags.pullSecret,
		"pull-secret",
		"",
		"Name of the file containing the pull secret",
	)
	return result
}

type createBundleCommand struct {
	flags struct {
		version    string
		arch       string
		outputDir  string
		pullSecret string
	}
}

func (c *createBundleCommand) run(cmd *cobra.Command, argv []string) error {
	var err error

	// Get the context:
	ctx := cmd.Context()

	// Get the dependencies from the context:
	logger := internal.LoggerFromContext(ctx)
	console := internal.ConsoleFromContext(ctx)

	// Check the flags:
	ok := true
	if c.flags.version == "" {
		console.Error("Version is mandatory")
		ok = false
	}
	if c.flags.arch == "" {
		console.Error("Architecture is mandatory")
		ok = false
	}
	if c.flags.outputDir == "" {
		console.Error("Output directory is mandatory")
		ok = false
	}
	if c.flags.pullSecret == "" {
		console.Error("Pull secret is mandatory")
		ok = false
	}
	if !ok {
		return exit.Error(1)
	}

	// Create and run the bundle creator:
	creator, err := internal.NewBundleCreator().
		SetLogger(logger).
		SetConsole(console).
		SetVersion(c.flags.version).
		SetArch(c.flags.arch).
		SetPullSecret(c.flags.pullSecret).
		SetOutputDir(c.flags.outputDir).
		Build()
	if err != nil {
		logger.Error(err, "Failed to create creator")
		return exit.Error(1)
	}
	err = creator.Run(ctx)
	if err != nil {
		logger.Error(err, "Failed to run creator")
		return exit.Error(1)
	}

	return nil
}
