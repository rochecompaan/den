package configdir

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rochecompaan/den/internal/manifest"
)

// ACLValidator is the existing secure-directory dependency set under its
// multi-binding name. It remains a data-only compatibility seam.
type ACLValidator = Dependencies

// Handle is a selected state directory with its rollback lifecycle.
type Handle = Selection

type bindingSource uint8

const (
	defaultSource bindingSource = iota
	explicitSource
	inheritedSource
)

type plannedBinding struct {
	spec     manifest.StateBinding
	path     string
	source   bindingSource
	writable []string
	denied   []string
}

// BindingPlan selects all bindings and rejects aliases before opening paths.
type BindingPlan struct {
	bindings    []plannedBinding
	runtimeHome string
}

// PlanBindings applies explicit, inherited, then default precedence using the
// original environment. It does not create or open a state directory.
func PlanBindings(specs []manifest.StateBinding, inherited map[string]string, runtimeHome string) (BindingPlan, error) {
	if runtimeHome == "" || !filepath.IsAbs(runtimeHome) || filepath.Clean(runtimeHome) != runtimeHome {
		return BindingPlan{}, errInvalid
	}
	plan := BindingPlan{bindings: make([]plannedBinding, 0, len(specs)), runtimeHome: runtimeHome}
	for _, spec := range specs {
		path, source, err := selectBindingPath(spec, inherited, runtimeHome)
		if err != nil {
			return BindingPlan{}, err
		}
		binding := plannedBinding{spec: spec, path: path, source: source}
		defaults := bindingDefaults(spec, runtimeHome)
		if source == defaultSource {
			binding.writable = defaults
		} else {
			binding.denied = defaults
		}
		for _, earlier := range plan.bindings {
			if binding.path != "" && earlier.path != "" && pathsOverlap(binding.path, earlier.path) {
				return BindingPlan{}, errOverlap
			}
		}
		plan.bindings = append(plan.bindings, binding)
	}
	return plan, nil
}

func selectBindingPath(spec manifest.StateBinding, inherited map[string]string, home string) (string, bindingSource, error) {
	value, source := "", defaultSource
	if spec.ExplicitPath != nil {
		value, source = *spec.ExplicitPath, explicitSource
	} else if inherited != nil {
		if inheritedValue, ok := inherited[spec.InheritedEnvironment]; ok {
			value, source = inheritedValue, inheritedSource
		}
	}
	if source == defaultSource {
		value = spec.DefaultPath
	}
	if value == "" {
		return "", source, nil
	}
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", source, errInvalid
	}
	info, err := os.Lstat(value)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", source, errInvalid
	}
	if err != nil && !os.IsNotExist(err) {
		return "", source, errInvalid
	}
	canonical, err := canonicalProspective(value)
	if err != nil {
		return "", source, errInvalid
	}
	return canonical, source, nil
}

func bindingDefaults(spec manifest.StateBinding, home string) []string {
	if len(spec.DefaultWritablePaths) != 0 {
		return append([]string(nil), spec.DefaultWritablePaths...)
	}
	if spec.DefaultPath == "" {
		return claudeDefaultPaths(home)
	}
	return []string{directoryPolicyPath(spec.DefaultPath)}
}

// Open securely opens every selected concrete directory. On a later error it
// rolls back all prior newly-created directories before returning that error.
func (p BindingPlan) Open(platform string, acl ACLValidator) ([]*Handle, error) {
	if platform == "" {
		platform = runtime.GOOS
	}
	if platform != "linux" && platform != "darwin" {
		return nil, errors.New("invalid platform")
	}
	if len(acl.ProtectedHomes) == 0 {
		acl.ProtectedHomes = []string{p.runtimeHome}
	}
	handles := make([]*Handle, 0, len(p.bindings))
	for _, binding := range p.bindings {
		if binding.path == "" {
			protected, err := expandProtectedPatterns(acl.ProtectedPathPatterns, acl.ProtectedHomes)
			if err != nil {
				for index := len(handles) - 1; index >= 0; index-- {
					_ = handles[index].Rollback()
				}
				return nil, errInvalid
			}
			handles = append(handles, &Selection{Mode: Default, WritablePaths: binding.writable, DeniedDefaultPaths: binding.denied, ProtectedPaths: protected, binding: binding.spec, source: binding.source})
			continue
		}
		selection, err := selectCustom(binding.path, p.runtimeHome, acl.ProtectedPathPatterns, Dependencies{ACLProbe: acl.ACLProbe, ProtectedHomes: acl.ProtectedHomes})
		if err != nil {
			for index := len(handles) - 1; index >= 0; index-- {
				_ = handles[index].Rollback()
			}
			return nil, err
		}
		selection.WritablePaths = []string{directoryPolicyPath(selection.CanonicalPath)}
		selection.DeniedDefaultPaths = binding.denied
		if binding.source == defaultSource {
			selection.Mode = Default
			selection.WritablePaths = binding.writable
		}
		selection.binding, selection.source = binding.spec, binding.source
		handles = append(handles, &selection)
	}
	return handles, nil
}

func (p BindingPlan) WritablePaths() []string {
	return bindingPaths(p.bindings, func(binding plannedBinding) []string { return binding.writable })
}
func (p BindingPlan) DeniedWritePaths() []string {
	return bindingPaths(p.bindings, func(binding plannedBinding) []string { return binding.denied })
}

func bindingPaths(bindings []plannedBinding, choose func(plannedBinding) []string) []string {
	var result []string
	for _, binding := range bindings {
		result = append(result, choose(binding)...)
	}
	return result
}

// Exports identifies the manifest exports attached to this selected handle.
func (s Selection) Exports() []manifest.StateExport {
	return append([]manifest.StateExport(nil), s.binding.Exports...)
}
func (s Selection) ExportDefault() bool { return s.source == defaultSource }
func (s Selection) Close() error        { return s.Rollback() }
