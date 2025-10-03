package providers

import (
	"context"
	"fmt"
	"io/fs"
	"sync"

	"github.com/kyverno/kyverno-http-authorizer/pkg/data"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"

	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno/api/policies.kyverno.io/v1alpha1"
	"github.com/kyverno/pkg/ext/file"
	"github.com/kyverno/pkg/ext/resource/convert"
	"github.com/kyverno/pkg/ext/resource/loader"
	"github.com/kyverno/pkg/ext/yaml"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
)

var (
	vpol = v1alpha1.SchemeGroupVersion.WithKind("ValidatingPolicy")
)

func defaultLoader(_fs func() (fs.FS, error)) (loader.Loader, error) {
	if _fs == nil {
		_fs = data.Crds
	}
	crdsFs, err := _fs()
	if err != nil {
		return nil, err
	}
	return loader.New(openapiclient.NewLocalCRDFiles(crdsFs))
}

var DefaultLoader = sync.OnceValues(func() (loader.Loader, error) { return defaultLoader(nil) })

type FsProvider struct {
	vpolCompiler vpolcompiler.Compiler
	logger       *logrus.Logger
	fs           fs.FS
}

func NewFsProvider(logger *logrus.Logger, vpolCompiler vpolcompiler.Compiler, fs fs.FS) *FsProvider {
	return &FsProvider{
		vpolCompiler: vpolCompiler,
		fs:           fs,
		logger:       logger,
	}
}

func (p *FsProvider) CompiledPolicies(ctx context.Context) ([]engine.CompiledPolicy, error) {
	raw, err := p.loadAll()
	if err != nil {
		return nil, err
	}

	var policies []engine.CompiledPolicy
	for _, pol := range raw {
		compiled, err := p.vpolCompiler.Compile(pol)
		if err != nil {
			// TODO: handle single policy errors
			continue
		}
		policies = append(policies, compiled)
	}

	return policies, nil
}

func (p *FsProvider) Policies(ctx context.Context) ([]*v1alpha1.ValidatingPolicy, error) {
	return p.loadAll()
}

func (p *FsProvider) loadAll() ([]*v1alpha1.ValidatingPolicy, error) {
	var policies []*v1alpha1.ValidatingPolicy
	entries, err := fs.ReadDir(p.fs, ".")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		// TODO: recursive loading
		// TODO: json support
		if entry.IsDir() || !file.IsYaml(entry.Name()) {
			continue
		}
		bytes, err := fs.ReadFile(p.fs, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", entry.Name(), err)
		}
		documents, err := yaml.SplitDocuments(bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to split documents: %w", err)
		}
		ldr, err := DefaultLoader()
		if err != nil {
			return nil, fmt.Errorf("failed to load CRDs: %w", err)
		}
		for _, document := range documents {
			gvk, untyped, err := ldr.Load(document)
			if err != nil {
				p.logger.Error("error loading document from external provider", err)
				continue
			}
			if gvk != vpol {
				p.logger.Error("skipping the loaded document because it wasn't a validating policy, got: ", gvk.String())
				continue
			}
			typed, err := convert.To[v1alpha1.ValidatingPolicy](untyped)
			if err != nil {
				return nil, fmt.Errorf("failed to convert to ValidatingPolicy: %w", err)
			}

			policies = append(policies, typed)
		}
	}
	return policies, nil
}
