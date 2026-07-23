package config

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveContext maps an alias name to a kubeconfig context.
// If name is not an alias key, it is returned unchanged.
// aliasUsed is the matched alias key (empty when name was already a raw context).
func ResolveContext(name string, aliases map[string]string) (resolved, aliasUsed string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	if len(aliases) == 0 {
		return name, ""
	}
	if target, ok := aliases[name]; ok {
		t := strings.TrimSpace(target)
		if t == "" {
			return name, ""
		}
		return t, name
	}
	for k, v := range aliases {
		if strings.EqualFold(k, name) {
			t := strings.TrimSpace(v)
			if t == "" {
				return name, ""
			}
			return t, k
		}
	}
	return name, ""
}

// SetAlias writes aliases.<name> = kubeContext and persists config.
func SetAlias(name, kubeContext string) (File, error) {
	name = strings.TrimSpace(name)
	kubeContext = strings.TrimSpace(kubeContext)
	if err := validateAliasName(name); err != nil {
		return File{}, err
	}
	if kubeContext == "" {
		return File{}, fmt.Errorf("kube context cannot be empty")
	}
	f, err := LoadFile()
	if err != nil {
		return File{}, err
	}
	if f.Aliases == nil {
		f.Aliases = map[string]string{}
	}
	f.Aliases[name] = kubeContext
	if err := SaveFile(f); err != nil {
		return File{}, err
	}
	return f, nil
}

// UnsetAlias removes an alias and persists config.
func UnsetAlias(name string) (File, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return File{}, fmt.Errorf("alias name cannot be empty")
	}
	f, err := LoadFile()
	if err != nil {
		return File{}, err
	}
	if f.Aliases == nil {
		return File{}, fmt.Errorf("alias %q not set", name)
	}
	if _, ok := f.Aliases[name]; !ok {
		// case-insensitive delete
		found := ""
		for k := range f.Aliases {
			if strings.EqualFold(k, name) {
				found = k
				break
			}
		}
		if found == "" {
			return File{}, fmt.Errorf("alias %q not set", name)
		}
		name = found
	}
	delete(f.Aliases, name)
	if len(f.Aliases) == 0 {
		f.Aliases = nil
	}
	if err := SaveFile(f); err != nil {
		return File{}, err
	}
	return f, nil
}

// AliasLines returns sorted "name → context" lines for display.
func AliasLines(aliases map[string]string) []string {
	if len(aliases) == 0 {
		return nil
	}
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s → %s", k, aliases[k]))
	}
	return out
}

func validateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("alias name cannot be empty")
	}
	if strings.ContainsAny(name, " \t\n/") {
		return fmt.Errorf("alias name %q must not contain spaces or /", name)
	}
	if len(name) > 63 {
		return fmt.Errorf("alias name too long")
	}
	return nil
}
