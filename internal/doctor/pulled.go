package doctor

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func readPulledProviderCount(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var m map[string]string
	if err := yaml.Unmarshal(data, &m); err != nil {
		return 0, err
	}
	n := 0
	for k, v := range m {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			n++
		}
	}
	return n, nil
}
