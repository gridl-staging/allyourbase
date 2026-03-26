package templates

import "sort"

// DomainTemplate defines a domain-specific scaffold template.
type DomainTemplate interface {
	Name() string
	Schema() string
	SeedData() string
	ClientCode() map[string]string
	Readme() string
}

var registry = map[string]DomainTemplate{}

// Register adds a domain template to the package registry.
func Register(dt DomainTemplate) {
	if dt == nil {
		panic("templates: cannot register nil template")
	}
	name := dt.Name()
	if name == "" {
		panic("templates: template name cannot be empty")
	}
	if _, exists := registry[name]; exists {
		panic("templates: duplicate template registration: " + name)
	}
	registry[name] = dt
}

// Get returns a domain template by name.
func Get(name string) (DomainTemplate, bool) {
	dt, ok := registry[name]
	return dt, ok
}

// Names returns all registered template names in deterministic order.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
