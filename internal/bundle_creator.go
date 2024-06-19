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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	dreference "github.com/distribution/distribution/v3/reference"
	"github.com/go-logr/logr"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"github.com/jhernand/upgrade-tool/internal/exit"
	"github.com/jhernand/upgrade-tool/internal/jq"
	jqtool "github.com/jhernand/upgrade-tool/internal/jq"
)

// BundleCreatorBuilder contains the data and logic needed to create an object that knows how to
// create an upgrade bundle file. Don't create instances of this type directly, use the
// NewBundleCreator function instead.
type BundleCreatorBuilder struct {
	logger     logr.Logger
	console    *Console
	version    string
	arch       string
	outputDir  string
	pullSecret string
}

// BundleCreator knows how to create an upgrade bundle file. Don't create intances of this type
// directly, use the NewBundleCreator function instead.
type BundleCreator struct {
	logger     logr.Logger
	console    *Console
	jq         *jqtool.Tool
	version    string
	arch       string
	outputDir  string
	pullSecret string
}

// NewBundleCreator creates a builder that can then be used to create and configure a bundle
// creator.
func NewBundleCreator() *BundleCreatorBuilder {
	return &BundleCreatorBuilder{}
}

// SetLogger sets the logger that the bundle creator will use to write messages to the log. This is
// mandatory.
func (b *BundleCreatorBuilder) SetLogger(value logr.Logger) *BundleCreatorBuilder {
	b.logger = value
	return b
}

// SetConsole sets the console that the bundle creator will use to write friendly messages to the
// console. This is mandatory.
func (b *BundleCreatorBuilder) SetConsole(value *Console) *BundleCreatorBuilder {
	b.console = value
	return b
}

// SetVersion sets the OpenShift version of the bundle, for example '4.13.4'. This is mandatory.
func (b *BundleCreatorBuilder) SetVersion(value string) *BundleCreatorBuilder {
	b.version = value
	return b
}

// SetArch sets the architecture of the bundle, for example 'x86_64'. This is mandatory.
func (b *BundleCreatorBuilder) SetArch(value string) *BundleCreatorBuilder {
	b.arch = value
	return b
}

// SetOutputDir sets the directory where the bundle creator will write the bundle files. This is
// mandatory.
func (b *BundleCreatorBuilder) SetOutputDir(value string) *BundleCreatorBuilder {
	b.outputDir = value
	return b
}

// SetPullSecret sets the file that contains the pull secret that the bundle creator will use to
// authenticate to the image registry in order to pull the images. This is mandatory.
func (b *BundleCreatorBuilder) SetPullSecret(value string) *BundleCreatorBuilder {
	b.pullSecret = value
	return b
}

// Build uses the data stored in the builder to create and configure a new bundle creator.
func (b *BundleCreatorBuilder) Build() (result *BundleCreator, err error) {
	// Check parameters:
	if b.logger.GetSink() == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.console == nil {
		err = errors.New("console is mandatory")
		return
	}
	if b.version == "" {
		err = errors.New("version is mandatory")
		return
	}
	if b.arch == "" {
		err = errors.New("architecture is mandatory")
		return
	}
	if b.outputDir == "" {
		err = errors.New("output directory is mandatory")
		return
	}
	if b.pullSecret == "" {
		err = errors.New("pull secret is mandatory")
		return
	}

	// Create the jq tool:
	jq, err := jq.NewTool().
		SetLogger(b.logger).
		Build()
	if err != nil {
		return
	}

	// Create and populate the object:
	result = &BundleCreator{
		logger:     b.logger,
		console:    b.console,
		jq:         jq,
		version:    b.version,
		arch:       b.arch,
		outputDir:  b.outputDir,
		pullSecret: b.pullSecret,
	}
	return
}

func (c *BundleCreator) Run(ctx context.Context) error {
	// Determine the cache directories:
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		c.console.Error(
			"Failed to find user cache directory: %v",
			err,
		)
		return exit.Error(1)
	}
	tmpDir := filepath.Join(
		cacheDir,
		"upgrade-tool",
		fmt.Sprintf("%s-%s", c.version, c.arch),
	)
	err = c.createDir(tmpDir)
	if err != nil {
		c.console.Error(
			"Failed to create bundle directory '%s': %v",
			tmpDir, err,
		)
		return exit.Error(1)
	}

	// Find the images:
	c.console.Info("Finding images ...")
	release, images, err := c.findImages(ctx)
	if err != nil {
		c.console.Error("Failed to find release images: %v", err)
		return exit.Error(1)
	}
	c.logger.Info(
		"Found images",
		"release", release,
		"images", len(images),
	)

	// Create the registry:
	c.console.Info("Starting registry ...")
	registry, err := c.createRegistry(ctx, tmpDir)
	if err != nil {
		c.console.Error("Failed to start registry: %v", err)
		return exit.Error(1)
	}

	// Download the images:
	err = c.downloadImages(registry, release, images)
	if err != nil {
		c.console.Info("registry：%s，release: %s, img: %s", registry, release, images)
		c.console.Error("Failed to download images: %v", err)
		return exit.Error(1)
	}

	// Stop the registry:
	c.console.Info("Stopping registry ...")
	err = registry.Stop(ctx)
	if err != nil {
		c.console.Error("Failed to stop registry: %v", err)
		return exit.Error(1)
	}

	// Write the metadata:
	c.console.Info("Writing metadata ...")
	metadata := &Metadata{
		Version: c.version,
		Arch:    c.arch,
		Release: release,
		Images:  maps.Values(images),
	}
	err = c.writeMetadata(metadata, tmpDir)
	if err != nil {
		c.console.Error("Failed to write metadata: %v", err)
		return exit.Error(1)
	}

	// Write the bundle:
	c.console.Info("Writing bundle to '%s' ...", c.bundleFile())
	err = c.writeBundle(tmpDir)
	if err != nil {
		c.console.Error("Failed to write bundle: %v", err)
		return exit.Error(1)
	}

	// Write the digest:
	c.console.Info("Writing digest to '%s' ...", c.digestFile())
	err = c.writeDigest()
	if err != nil {
		c.console.Error("Failed to write digest: %v", err)
		return exit.Error(1)
	}

	// Write the manifest:
	c.console.Info("Writing manifest to '%s' ...", c.manifestFile())
	err = c.writeManifest()
	if err != nil {
		c.console.Error("Failed to write manifest: %v", err)
		return exit.Error(1)
	}

	return nil
}

func (c *BundleCreator) createRegistry(ctx context.Context,
	dir string) (registry *Registry, err error) {
	registry, err = NewRegistry().
		SetLogger(c.logger).
		SetAddress("localhost:5001").
		SetRoot(dir).
		Build()
	if err != nil {
		return
	}
	err = registry.Start(ctx)
	return
}

func (c *BundleCreator) findImages(ctx context.Context) (release string, images map[string]string,
	err error) {
	release = fmt.Sprintf("%s:%s-%s", bundleCreatorReleaseRepo, c.version, c.arch)
	path, err := exec.LookPath("oc")
	if err != nil {
		return
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := exec.Cmd{
		Path: path,
		Args: []string{
			"oc", "adm", "release", "info",
			"--output=json",
			release,
		},
		Stdout: stdout,
		Stderr: stderr,
	}
	err = cmd.Run()
	c.logger.Info(
		"Executed 'oc' command",
		"args", cmd.Args,
		"stdout", cmd.String(),
		"stderr", cmd.String(),
		"code", cmd.ProcessState.ExitCode(),
	)
	if err != nil {
		return
	}
	var digest string
	err = c.jq.QueryBytes(
		`.digest`,
		stdout.Bytes(), &digest,
	)
	if err != nil {
		return
	}
	release = fmt.Sprintf("%s@%s", bundleCreatorReleaseRepo, digest)
	type Tag struct {
		Tag string `json:"tag"`
		Ref string `json:"ref"`
	}
	var tags []Tag
	err = c.jq.QueryBytes(
		`[.references.spec.tags[] | {
			"tag": .name,
			"ref": .from.name
		}]`,
		stdout.Bytes(), &tags,
	)
	if err != nil {
		return
	}
	images = map[string]string{}
	for _, tag := range tags {
		images[tag.Tag] = tag.Ref
	}
	return
}

func (c *BundleCreator) downloadImages(registry *Registry, release string,
	images map[string]string) error {
	// Save the TLS certificate of the registry to a temporary directory, so that we can later
	// pass it to the '--dest-cert-dir' of the skopeo command.
	cert, _ := registry.Certificate()
	certs, err := os.MkdirTemp("", "*.skopeo")
	if err != nil {
		return err
	}
	defer func() {
		err := os.RemoveAll(certs)
		if err != nil {
			c.logger.Error(
				err,
				"Failed to remove skopeo temporary certificates directory",
				"dir", certs,
			)
		}
	}()
	file := filepath.Join(certs, "tls.crt")
	err = os.WriteFile(file, cert, 0400)
	if err != nil {
		return err
	}

	// Download the release image:
	dst, err := c.dstRef(release, registry)
	if err != nil {
		return err
	}
	c.console.Info("Downloading release image '%s' ...", release)
	err = c.downloadImage(certs, release, dst)
	if err != nil {
		return err
	}

	// Download the images:
	tags := maps.Keys(images)
	slices.Sort(tags)
	for i, tag := range tags {
		ref := images[tag]
		c.console.Info(
			"Downloading payload image %d of %d (%s) ...",
			i+1, len(tags), tag,
		)
		dst, err := c.dstRef(ref, registry)
		if err != nil {
			return err
		}
		err = c.downloadImage(certs, ref, dst)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *BundleCreator) dstRef(src string, registry *Registry) (dst string, err error) {
	ref, err := dreference.ParseNamed(src)
	if err != nil {
		return
	}
	path := dreference.Path(ref)
	tagged, ok := ref.(dreference.Tagged)
	var tag string
	if ok {
		tag = tagged.Tag()
	} else {
		diggested, ok := ref.(dreference.Digested)
		if ok {
			tag = diggested.Digest().Hex()
		}
	}
	dst = fmt.Sprintf("%s/%s:%s", registry.Address(), path, tag)
	return
}

func (c *BundleCreator) downloadImage(certs string, src, dst string) error {
	path, err := exec.LookPath("skopeo")
	if err != nil {
		return err
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := exec.Cmd{
		Path: path,
		Args: []string{
			"skopeo", "copy",
			fmt.Sprintf("--src-authfile=%s", c.pullSecret),
			fmt.Sprintf("--dest-cert-dir=%s", certs),
			fmt.Sprintf("docker://%s", src),
			fmt.Sprintf("docker://%s", dst),
		},
		Stdout: stdout,
		Stderr: stderr,
	}
	err = cmd.Run()
	c.logger.Info(
		"Executed 'skopeo' command",
		"args", cmd.Args,
		"stdout", stdout.String(),
		"stderr", stderr.String(),
		"code", cmd.ProcessState.ExitCode(),
	)
	return err
}

func (c *BundleCreator) writeMetadata(metadata *Metadata, dir string) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	file := filepath.Join(dir, "metadata.json")
	return os.WriteFile(file, data, 0644)
}

func (c *BundleCreator) writeBundle(dir string) error {
	bundle := c.bundleFile()
	path, err := exec.LookPath("tar")
	if err != nil {
		return err
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := exec.Cmd{
		Path: path,
		Args: []string{
			"tar",
			fmt.Sprintf("--directory=%s", dir),
			"--create",
			fmt.Sprintf("--file=%s", bundle),
			"metadata.json",
			"docker",
		},
		Stdout: stdout,
		Stderr: stderr,
	}
	err = cmd.Run()
	c.logger.Info(
		"Executed 'tar' command",
		"args", cmd.Args,
		"stdout", stdout.String(),
		"stderr", stderr.String(),
		"code", cmd.ProcessState.ExitCode(),
	)
	return err
}

func (c *BundleCreator) writeDigest() error {
	bundle := c.bundleFile()
	digest := c.digestFile()
	hash := sha256.New()
	reader, err := os.Open(bundle)
	if err != nil {
		return err
	}
	defer func() {
		err := reader.Close()
		if err != nil {
			c.logger.Error(
				err,
				"Failed to close bundle file",
				"file", bundle,
			)
		}
	}()
	_, err = io.Copy(hash, reader)
	if err != nil {
		return err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	file, err := os.OpenFile(digest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		err := file.Close()
		if err != nil {
			c.logger.Error(
				err,
				"Failed to close digest file",
				"file", digest,
			)
		}
	}()
	_, err = fmt.Fprintf(file, "%s  %s\n", sum, filepath.Base(bundle))
	if err != nil {
		return err
	}
	return nil
}

func (c *BundleCreator) writeManifest() error {
	content, err := TemplatesFS.ReadFile("templates/manifest.yaml")
	if err != nil {
		return err
	}
	manifest := c.manifestFile()
	err = os.WriteFile(manifest, content, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (c *BundleCreator) bundleFile() string {
	return c.outputBase() + ".tar"
}

func (c *BundleCreator) digestFile() string {
	return c.outputBase() + ".sha256"
}

func (c *BundleCreator) manifestFile() string {
	return c.outputBase() + ".yaml"
}

func (c *BundleCreator) outputBase() string {
	name := fmt.Sprintf("upgrade-%s-%s", c.version, c.arch)
	return filepath.Join(c.outputDir, name)
}

func (c *BundleCreator) createDir(dir string) error {
	err := os.MkdirAll(dir, 0700)
	if errors.Is(err, os.ErrExist) {
		err = nil
	}
	return err
}

const bundleCreatorReleaseRepo = "quay.io/openshift-release-dev/ocp-release"
