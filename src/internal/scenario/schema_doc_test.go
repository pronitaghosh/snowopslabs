// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/sagar2395/snowopslabs/pkg/checks"
	pkgscenario "github.com/sagar2395/snowopslabs/pkg/scenario"
)

// schemaRoots are the structs a scenario.yaml is unmarshalled into. Together
// their yaml tags are the field vocabulary the validator accepts.
var schemaRoots = []any{
	pkgscenario.Scenario{},
	pkgscenario.Stage{},
	pkgscenario.Prerequisites{},
	pkgscenario.Component{},
	pkgscenario.Explore{},
	pkgscenario.ExploreURL{},
	pkgscenario.ExploreCommand{},
	pkgscenario.Reference{},
	pkgscenario.Snippet{},
	pkgscenario.Parameter{},
	pkgscenario.Requirements{},
	checks.Check{},
}

// schemaFields returns the yaml field names the schema defines, plus the
// subset whose values are free-form maps (their sub-keys are data, not
// fields — e.g. the --set entries under a component's `set:`).
func schemaFields() (fields, freeMaps map[string]bool) {
	fields = map[string]bool{}
	freeMaps = map[string]bool{}
	for _, root := range schemaRoots {
		t := reflect.TypeOf(root)
		for i := range t.NumField() {
			f := t.Field(i)
			name := strings.SplitN(f.Tag.Get("yaml"), ",", 2)[0]
			if name == "" || name == "-" {
				continue
			}
			fields[name] = true
			if f.Type.Kind() == reflect.Map {
				freeMaps[name] = true
			}
		}
	}
	return fields, freeMaps
}

// yamlFence matches a ```yaml fenced block and captures its body.
var yamlFence = regexp.MustCompile("(?s)```yaml\n(.*?)```")

// documentedFields collects every mapping key used in the ```yaml examples
// of the scenario schema reference. Each block is parsed as YAML, so literal
// text inside block scalars (such as the example ConfigMap in the snippets
// section) is not mistaken for fields. Keys under a free-form map field are
// data, not fields, and are skipped.
func documentedFields(t *testing.T, freeMaps map[string]bool) map[string]bool {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// This file is in src/internal/scenario; the repo root is three levels up.
	doc := filepath.Join(filepath.Dir(thisFile), "..", "..", "..",
		"docs", "reference", "scenario-schema.md")
	data, err := os.ReadFile(doc) //nolint:gosec // test reads the repo's own doc
	if err != nil {
		t.Fatalf("reading the scenario schema reference: %v", err)
	}
	blocks := yamlFence.FindAllSubmatch(data, -1)
	if len(blocks) == 0 {
		t.Fatal("no ```yaml blocks found in docs/reference/scenario-schema.md")
	}
	fields := map[string]bool{}
	for _, b := range blocks {
		var doc yaml.Node
		if err := yaml.Unmarshal(b[1], &doc); err != nil {
			t.Fatalf("docs/reference/scenario-schema.md has a ```yaml block that does not parse: %v", err)
		}
		// Walk the node tree rather than decoding into maps: the reference
		// intentionally repeats a key to show alternatives (platformValues),
		// which map decoding would reject as a duplicate.
		collectKeys(&doc, fields, freeMaps)
	}
	return fields
}

func collectKeys(n *yaml.Node, fields, freeMaps map[string]bool) {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			collectKeys(c, fields, freeMaps)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			name := ""
			if k.Kind == yaml.ScalarNode {
				name = k.Value
				fields[name] = true
			}
			// The sub-keys of a free-form map are data (e.g. --set
			// entries), not schema fields.
			if !freeMaps[name] {
				collectKeys(v, fields, freeMaps)
			}
		}
	case yaml.AliasNode:
		collectKeys(n.Alias, fields, freeMaps)
	}
	// Scalar nodes (including block-scalar text such as the example ConfigMap
	// in the snippets section) hold no fields.
}

// TestScenarioSchemaDocMatchesSchema keeps docs/reference/scenario-schema.md
// in step with the fields the validator accepts: a field added to the schema
// must be documented, and a documented field removed from the schema must go
// from the reference too. It runs with the rest of `make test-go`.
func TestScenarioSchemaDocMatchesSchema(t *testing.T) {
	fields, freeMaps := schemaFields()
	if len(fields) == 0 {
		t.Fatal("no schema fields collected; the reflection over schemaRoots is broken")
	}
	documented := documentedFields(t, freeMaps)

	var missing []string
	for f := range fields {
		if !documented[f] {
			missing = append(missing, f)
		}
	}
	sort.Strings(missing)
	for _, f := range missing {
		t.Errorf("schema field %q is not documented in docs/reference/scenario-schema.md", f)
	}

	var stale []string
	for f := range documented {
		if !fields[f] {
			stale = append(stale, f)
		}
	}
	sort.Strings(stale)
	for _, f := range stale {
		t.Errorf("docs/reference/scenario-schema.md documents %q, which the schema does not define", f)
	}
}
