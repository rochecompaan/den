package launch

import "github.com/rochecompaan/den/internal/configdir"

// StateInputs are generic state exports and Fence filesystem inputs.
type StateInputs struct {
	Environment      map[string]string
	Arguments        []string
	WritablePaths    []string
	DeniedWritePaths []string
}

// StateInputsFrom applies exports in manifest binding and export order.
func StateInputsFrom(handles []*configdir.Handle) StateInputs {
	inputs := StateInputs{Environment: make(map[string]string)}
	for _, handle := range handles {
		inputs.WritablePaths = append(inputs.WritablePaths, handle.WritablePaths...)
		inputs.DeniedWritePaths = append(inputs.DeniedWritePaths, handle.DeniedDefaultPaths...)
		if handle.CanonicalPath == "" {
			continue
		}
		for _, export := range handle.Exports() {
			if handle.ExportDefault() && !export.ExportDefault {
				continue
			}
			if export.Kind == "environment" {
				inputs.Environment[export.Name] = handle.CanonicalPath
			} else {
				inputs.Arguments = append(inputs.Arguments, export.Name, handle.CanonicalPath)
			}
		}
	}
	return inputs
}

func closeStateHandles(handles []*configdir.Handle) error {
	var first error
	for index := len(handles) - 1; index >= 0; index-- {
		if err := handles[index].Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func commitStateHandles(handles []*configdir.Handle) {
	for _, handle := range handles {
		handle.Commit()
	}
}

func protectedStatePaths(handles []*configdir.Handle) []string {
	var paths []string
	for _, handle := range handles {
		paths = append(paths, handle.ProtectedPaths...)
	}
	return paths
}
