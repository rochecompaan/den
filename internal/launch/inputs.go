package launch

import (
	"sort"

	"github.com/rochecompaan/den/internal/environment"
	"github.com/rochecompaan/den/internal/manifest"
)

// ChildInputs are the complete controlled environment and argv for one agent.
type ChildInputs struct {
	Environment []string
	Arguments   []string
}

// BuildChildInputs applies adapter-owned environment and argument inputs.
func BuildChildInputs(host []string, controlled environment.Controlled, agent manifest.Agent, state StateInputs, userArgs []string) ChildInputs {
	return buildChildInputs(environment.Build(environment.Scrub(host, agent.Environment.Scrub), controlled), agent, state, userArgs)
}

func buildChildInputs(childEnvironment []string, agent manifest.Agent, state StateInputs, userArgs []string) ChildInputs {
	childEnvironment = overwriteEnvironment(childEnvironment, state.Environment)
	childEnvironment = overwriteEnvironment(childEnvironment, agent.Environment.Set)
	if agent.PackageDirectory != nil {
		childEnvironment = environment.Overwrite(childEnvironment, agent.PackageDirectory.Name, agent.PackageDirectory.Value)
	}
	arguments := append([]string(nil), agent.MandatoryArgs...)
	if agent.SecurityAdapter != nil {
		arguments = append(arguments, agent.SecurityAdapter.Arguments...)
	}
	arguments = append(arguments, agent.ResourceArgs...)
	arguments = append(arguments, state.Arguments...)
	arguments = append(arguments, userArgs...)
	return ChildInputs{Environment: childEnvironment, Arguments: arguments}
}

func overwriteEnvironment(values []string, replacements map[string]string) []string {
	names := make([]string, 0, len(replacements))
	for name := range replacements {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		values = environment.Overwrite(values, name, replacements[name])
	}
	return values
}
