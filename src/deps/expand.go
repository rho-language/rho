package deps

import (
	"fmt"

	"rho/internal/paths"
	"rho/src/yaml"
)

// ExpandManifestPaths resolves `${VAR}` and `${VAR:-default}` in the fields of a
// manifest that name a place on disk, before anything reads them.
//
// A manifest that has to name a sibling checkout says where it is relative to
// the workspace — `${RHO_WORKSPACE}/lib` — rather than as a path on the machine
// it was written on, which is true for one person and published to everyone. Only
// the fields that are paths are touched: a `run:` command or a description may
// contain `${...}` that belongs to a shell or to prose.
func ExpandManifestPaths(doc yaml.Value, dir string) error {
	var failure error
	scalar := func(holder map[string]yaml.Value, key string) {
		item, ok := holder[key]
		if !ok || item.Kind != yaml.KindString || failure != nil {
			return
		}
		expanded, err := paths.Expand(item.Str, dir)
		if err != nil {
			failure = fmt.Errorf("%s: %w", key, err)
			return
		}
		item.Str = expanded
		holder[key] = item
	}
	list := func(holder map[string]yaml.Value, key string) {
		for i, item := range holder[key].List {
			if item.Kind != yaml.KindString || failure != nil {
				continue
			}
			expanded, err := paths.Expand(item.Str, dir)
			if err != nil {
				failure = fmt.Errorf("%s: %w", key, err)
				return
			}
			holder[key].List[i].Str = expanded
		}
	}

	for _, entry := range doc.Map["depends"].List {
		if entry.Kind == yaml.KindMap {
			scalar(entry.Map, "path")
		}
	}
	if build := doc.Map["build"]; build.Kind == yaml.KindMap {
		list(build.Map, "include_dirs")
		list(build.Map, "source_dirs")
	}
	if connection := doc.Map["c_connection"]; connection.Kind == yaml.KindMap {
		scalar(connection.Map, "path")
		list(connection.Map, "include_dirs")
		for _, variant := range connection.Map["variants"].Map {
			if variant.Kind == yaml.KindMap {
				list(variant.Map, "include_dirs")
			}
		}
	}
	if cmake := doc.Map["exports"].Map["cmake"]; cmake.Kind == yaml.KindMap {
		list(cmake.Map, "include_dirs")
	}
	return failure
}

// ExpandPath resolves `${VAR}` and `${VAR:-default}` in one path a component read
// out of a manifest or a target spec, against the workspace of `base`. It is the
// same expansion a manifest's own path fields get.
func ExpandPath(value, base string) (string, error) {
	return paths.Expand(value, base)
}
