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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	dconfiguration "github.com/distribution/distribution/v3/configuration"
	dhandlers "github.com/distribution/distribution/v3/registry/handlers"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"

	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
)

// RegistryBuilder contains the data and logic needed to build a simple image registry server. Don't
// create instances of this type directly, use the NewRegistry function instead.
type RegistryBuilder struct {
	logger  logr.Logger
	address string
	root    string
	cert    []byte
	key     []byte
}

// Registry implements a simple registry server. Don't create instances of this type directly, use
// the NewRegistry function instead.
type Registry struct {
	logger   logr.Logger
	address  string
	root     string
	tmp      string
	cert     []byte
	key      []byte
	listener net.Listener
	server   *http.Server
}

// NewRegistry creates a builder that can then be used to configure and create a new registry
// server.
func NewRegistry() *RegistryBuilder {
	return &RegistryBuilder{}
}

// SetLogger sets the logger that the registry will use to write log messages. This is mandatory.
func (b *RegistryBuilder) SetLogger(value logr.Logger) *RegistryBuilder {
	b.logger = value
	return b
}

// SetAddress sets the address where the registry server will listen. This is mandatory.
func (b *RegistryBuilder) SetAddress(value string) *RegistryBuilder {
	b.address = value
	return b
}

// SetRoot sets the root of the directory tree where the registry will store the images. This is
// mandatory.
func (b *RegistryBuilder) SetRoot(value string) *RegistryBuilder {
	b.root = value
	return b
}

// SetCertificate sets the TLS certificate and key (in PEM format) that will be used by the server.
// This is optional. If not set then a self signed certificate will be generated.
func (b *RegistryBuilder) SetCertificate(cert, key []byte) *RegistryBuilder {
	b.cert = slices.Clone(cert)
	b.key = slices.Clone(key)
	return b
}

// Build uses the data stored in the builder to create a new registry.
func (b *RegistryBuilder) Build() (result *Registry, err error) {
	// Check parameters:
	if b.logger.GetSink() == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.address == "" {
		err = errors.New("address is mandatory")
		return
	}
	if b.root == "" {
		err = errors.New("root is mandatory")
		return
	}
	if b.cert != nil && b.key == nil {
		err = errors.New("key is mandatory when certificate is set")
		return
	}
	if b.key != nil && b.cert == nil {
		err = errors.New("certificate is mandatory when key is set")
		return
	}

	// Create the temporary directory:
	tmp, err := os.MkdirTemp("", "*.registry")
	if err != nil {
		return
	}

	// Generate the TLS certificate and key if needed:
	cert, key := b.cert, b.key
	if b.cert == nil && b.key == nil {
		cert, key, err = b.makeSelfSignedCert()
		if err != nil {
			return
		}
	}

	// Create and populate the object:
	result = &Registry{
		logger:  b.logger,
		address: b.address,
		root:    b.root,
		tmp:     tmp,
		cert:    cert,
		key:     key,
	}
	return
}

func (b *RegistryBuilder) makeSelfSignedCert() (certPEM, keyPEM []byte, err error) {
	host, _, err := net.SplitHostPort(b.address)
	if err != nil {
		return
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return
	}
	ips := make([]net.IP, len(addrs))
	for i, addr := range addrs {
		ips[i] = net.ParseIP(addr)
	}
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return
	}
	now := time.Now()
	spec := x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName: host,
		},
		DNSNames: []string{
			host,
		},
		IPAddresses: ips,
		NotBefore:   now,
		NotAfter:    now.Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}
	cert, err := x509.CreateCertificate(rand.Reader, &spec, &spec, &key.PublicKey, key)
	if err != nil {
		return
	}
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return
}

// Address returns the address where the registry is listening.
func (r *Registry) Address() string {
	return r.listener.Addr().String()
}

// Root returns the root directory of the registry.
func (r *Registry) Root() string {
	return r.root
}

// Certificate returns the TLS certificate and key used by the registry, in PEM format.
func (r *Registry) Certificate() (cert, key []byte) {
	cert = slices.Clone(r.cert)
	key = slices.Clone(r.key)
	return
}

// Start starts the registry.
func (r *Registry) Start(ctx context.Context) error {
	var err error

	// The registry server uses logrus for logging, but we want to redirect that to our logr
	// logger:
	logrusLogger := logrus.StandardLogger()
	logrusLogger.SetOutput(ioutil.Discard)
	logrusLogger.Hooks.Add(&registryLogrHook{
		logger: r.logger,
	})

	// Start the registry server:
	certFile := filepath.Join(r.tmp, "tls.crt")
	err = os.WriteFile(certFile, r.cert, 0400)
	if err != nil {
		return err
	}
	keyFile := filepath.Join(r.tmp, "tls.key")
	err = os.WriteFile(keyFile, r.key, 0400)
	if err != nil {
		return err
	}
	r.listener, err = net.Listen("tcp", r.address)
	if err != nil {
		return err
	}
	configObj := &dconfiguration.Configuration{}
	configObj.Storage = dconfiguration.Storage{
		"filesystem": dconfiguration.Parameters{
			"rootdirectory": r.root,
		},
	}
	configObj.HTTP.Secret = "42"
	configObj.HTTP.Addr = r.listener.Addr().String()
	configObj.HTTP.TLS.Certificate = certFile
	configObj.HTTP.TLS.Key = keyFile
	configObj.Catalog.MaxEntries = 100
	r.server = &http.Server{
		Handler: dhandlers.NewApp(ctx, configObj),
	}
	if err != nil {
		return err
	}
	go func() {
		err = r.server.ServeTLS(r.listener, certFile, keyFile)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.logger.Error(err, "Failed to serve")
		}
	}()

	return nil
}

// Stop stops the registry.
func (r *Registry) Stop(ctx context.Context) error {
	// Shutdown the server:
	err := r.server.Shutdown(ctx)
	if err != nil {
		return err
	}

	// Remore the temporary directory:
	err = os.RemoveAll(r.tmp)
	if err != nil {
		return err
	}

	return nil
}

// registryLogrHook is a logrus hook that sends the log messages to a logr logger.
type registryLogrHook struct {
	logger logr.Logger
}

var _ logrus.Hook = (*registryLogrHook)(nil)

func (h *registryLogrHook) Fire(entry *logrus.Entry) error {
	fields := make([]any, 2*len(entry.Data))
	i := 0
	for name, value := range entry.Data {
		fields[2*i] = name
		fields[2*i+1] = value
		i++
	}
	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel:
		h.logger.Error(nil, entry.Message, fields...)
	default:
		level := int(entry.Level) - 2
		h.logger.V(level).Info(entry.Message, fields...)
	}
	return nil
}

func (h *registryLogrHook) Levels() []logrus.Level {
	return logrus.AllLevels
}
