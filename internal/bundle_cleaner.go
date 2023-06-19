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

package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jhernand/upgrade-tool/internal/labels"
)

// BundleCleanerBuilder contains the data and logic needed to create bundle cleaners. Don't create
// instances of this type directly, use the NewBundleCleaner function instead.
type BundleCleanerBuilder struct {
	logger    logr.Logger
	client    clnt.Client
	node      string
	rootDir   string
	bundleDir string
}

// BundleCleaner removes the temporary files and directories used by the upgrade process. Don't
// create instances of this type directly, use the NewBundleCleaner function instead.
type BundleCleaner struct {
	logger    logr.Logger
	client    clnt.Client
	node      string
	rootDir   string
	bundleDir string
	crioTool  *CRIOTool
}

// NewBundleCleaner creates a builder that can then be used to configure and create bundle cleaners.
func NewBundleCleaner() *BundleCleanerBuilder {
	return &BundleCleanerBuilder{}
}

// SetLogger sets the logger that the cleaner will use to write log messages. This is mandatory.
func (b *BundleCleanerBuilder) SetLogger(value logr.Logger) *BundleCleanerBuilder {
	b.logger = value
	return b
}

// SetClient sets the Kubernetes API client that the cleaner will use to write the annotations and
// labels used to report progress and to update the state of the cleaning process. This is
// mandatory.
func (b *BundleCleanerBuilder) SetClient(value clnt.Client) *BundleCleanerBuilder {
	b.client = value
	return b
}

// SetNode sets the name of the node where the clenaner is running. The loader will add to this node
// the annotations and labels that indicate the progress and state of the loading process. This is
// mandatory.
func (b *BundleCleanerBuilder) SetNode(value string) *BundleCleanerBuilder {
	b.node = value
	return b
}

// SetRootDir sets the root directory. This is optional, and when specified all the other
// directories are relative to it. This is intended for running the cleaner in a privileged pod with
// the node root filesystem mounted in a regular directory.
func (b *BundleCleanerBuilder) SetRootDir(value string) *BundleCleanerBuilder {
	b.rootDir = value
	return b
}

// SetBundleDir sets the directory where the bundle has been extracted. If the directory doesn't
// exist the loader remove it completely. This is mandatory.
func (b *BundleCleanerBuilder) SetBundleDir(value string) *BundleCleanerBuilder {
	b.bundleDir = value
	return b
}

// Build uses the data stored in the builder to create and configure a new bundle cleaner.
func (b *BundleCleanerBuilder) Build() (result *BundleCleaner, err error) {
	// Check parameters:
	if b.logger.GetSink() == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.client == nil {
		err = errors.New("client is mandatory")
		return
	}
	if b.node == "" {
		err = errors.New("node name is mandatory")
		return
	}
	if b.bundleDir == "" {
		err = errors.New("bundle directory is mandatory")
		return
	}

	// Create the CRI-O tool:
	crioTool, err := NewCRIOTool().
		SetLogger(b.logger).
		SetRootDir(b.rootDir).
		Build()
	if err != nil {
		err = fmt.Errorf("failed to create CRI-O tool: %w", err)
		return
	}

	// Create and populate the object:
	result = &BundleCleaner{
		logger:    b.logger,
		client:    b.client,
		node:      b.node,
		rootDir:   b.rootDir,
		bundleDir: b.bundleDir,
		crioTool:  crioTool,
	}
	return
}

func (l *BundleCleaner) Run(ctx context.Context) error {
	// Clean the bundle directory:
	err := l.cleanBundleDir(ctx)
	if err != nil {
		return err
	}
	l.logger.Info("Cleaned bundle directory")

	// Clean the CRI-O configuration:
	err = l.cleanCRIO(ctx)
	if err != nil {
		return err
	}
	l.logger.Info("Cleaned CRI-O")

	// Write the node annotations that indicate the result:
	err = l.writeResult(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *BundleCleaner) cleanBundleDir(ctx context.Context) error {
	dir := c.absolutePath(c.bundleDir)
	err := os.RemoveAll(dir)
	if err != nil {
		return err
	}
	c.logger.Info(
		"Removed bundle directory",
		"dir", dir,
	)
	tmp := fmt.Sprintf("%s.tmp", dir)
	err = os.RemoveAll(tmp)
	if err != nil {
		return err
	}
	c.logger.Info(
		"Removed bundle directory",
		"dir", dir,
	)
	return nil
}

func (c *BundleCleaner) absolutePath(relPath string) string {
	absPath := relPath
	if c.rootDir != "" {
		absPath = filepath.Join(c.rootDir, relPath)
	}
	return absPath
}

func (c *BundleCleaner) cleanCRIO(ctx context.Context) error {
	// Remove the configuration files:
	err := c.crioTool.RemoveMirrorConf()
	if err != nil {
		return err
	}
	err = c.crioTool.RemovePinConf()
	if err != nil {
		return err
	}

	// Reload the service:
	return c.crioTool.ReloadService(ctx)
}

func (c *BundleCleaner) writeResult(ctx context.Context) error {
	// Fetch the node:
	nodeObject := &corev1.Node{}
	nodeKey := clnt.ObjectKey{
		Name: c.node,
	}
	err := c.client.Get(ctx, nodeKey, nodeObject)
	if err != nil {
		return err
	}

	// Apply the patch:
	loadedText := strconv.FormatBool(true)
	nodeUpdate := nodeObject.DeepCopy()
	if nodeUpdate.Labels == nil {
		nodeUpdate.Labels = map[string]string{}
	}
	nodeUpdate.Labels[labels.BundleCleaned] = loadedText
	nodePatch := clnt.MergeFrom(nodeObject)
	err = c.client.Patch(ctx, nodeUpdate, nodePatch)
	if err != nil {
		return err
	}
	c.logger.V(1).Info(
		"Wrote success",
		"node", c.node,
	)
	return nil
}
