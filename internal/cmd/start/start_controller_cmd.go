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
	"os"
	sgnl "os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jhernand/upgrade-tool/internal"
	"github.com/jhernand/upgrade-tool/internal/exit"
)

// StartController creates and returns the `start controller` command.
func StartController() *cobra.Command {
	command := &startControllerCommand{}
	result := &cobra.Command{
		Use:   "controller",
		Short: "Starts the upgrade controller",
		Args:  cobra.NoArgs,
		RunE:  command.run,
	}
	flags := result.Flags()
	flags.StringVar(
		&command.flags.namespace,
		"namespace",
		"upgrade-tool",
		"Namespace where objects will be created",
	)
	return result
}

type startControllerCommand struct {
	logger logr.Logger
	flags  struct {
		namespace string
	}
}

func (c *startControllerCommand) run(cmd *cobra.Command, argv []string) error {
	var err error

	// Get the context:
	ctx := cmd.Context()

	// Get the dependencies from the context:
	c.logger = internal.LoggerFromContext(ctx)

	// Configure the controller runtime library to use the logger:
	ctrl.SetLogger(c.logger)

	// Check the flags:
	ok := true
	if c.flags.namespace == "" {
		c.logger.Error(nil, "Namespace is mandatory")
		ok = false
	}
	if !ok {
		return exit.Error(1)
	}

	// Create and start the controller:
	controller, err := internal.NewController().
		SetLogger(c.logger).
		SetNamespace(c.flags.namespace).
		Build()
	if err != nil {
		c.logger.Error(err, "Failed to create controller")
		return exit.Error(1)
	}
	err = controller.Start(ctx)
	if err != nil {
		c.logger.Error(err, "Failed to start controller")
		return exit.Error(1)
	}

	// Wait for the signal to stop:
	signals := make(chan os.Signal, 1)
	sgnl.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	signal := <-signals
	c.logger.Info(
		"Received stop signal",
		"signal", signal.String(),
	)

	// Stop the controller:
	err = controller.Stop(ctx)
	if err != nil {
		c.logger.Error(err, "Failed to stop controller")
		return exit.Error(1)
	}

	return nil
}
