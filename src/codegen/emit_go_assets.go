package codegen

import (
	"fmt"
	"sort"
	"strings"
)

// AssetFileName is the generated file that carries a program's module assets.
const AssetFileName = "rho_assets.go"

// EmitGoAssets renders the module assets a program carries and the lookup a
// Rho call to rho_asset(module, path) reaches. The key is the module's name and
// the asset's path inside it, so two modules may each have a templates/card.html.
func EmitGoAssets(assets map[string]string) string {
	keys := make([]string, 0, len(assets))
	for key := range assets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(goBanner(irModuleForBanner(), AssetFileName, "Generated Rho module assets."))
	b.WriteString("package main\n\n")
	b.WriteString("// The files each module of this program declared as its assets, by module\n")
	b.WriteString("// and path, embedded at build time.\n")
	b.WriteString("var rhoAssetFiles = map[string]string{\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "\t%q: %s,\n", key, goQuoted(assets[key]))
	}
	b.WriteString("}\n\n")
	b.WriteString("// rhoAsset is one module's asset, or \"\" when the module declares none by\n")
	b.WriteString("// that path.\n")
	b.WriteString("func rhoAsset(module string, path string) string {\n")
	b.WriteString("\treturn rhoAssetFiles[module+\"/\"+strings.TrimPrefix(path, \"./\")]\n")
	b.WriteString("}\n\n")
	b.WriteString(goAssetDirSource)
	return strings.Replace(b.String(), "package main\n\n", "package main\n\nimport (\n\t\"crypto/sha256\"\n\t\"encoding/hex\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"sort\"\n\t\"strings\"\n)\n\n", 1)
}

// goAssetDirSource is rho_asset_dir(module): the module's assets as files, for
// a reader that takes a path — a YAML document read where it lies, a directory
// of steps. They are written once per content under the user's cache, so two
// builds of a module never share a directory and a run never writes twice.
const goAssetDirSource = `// rhoAssetDir is a directory holding one module's assets as files, or "" when
// the module declares none or they cannot be written.
func rhoAssetDir(module string) string {
	prefix := module + "/"
	names := []string{}
	for key := range rhoAssetFiles {
		if strings.HasPrefix(key, prefix) {
			names = append(names, key)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	sum := sha256.New()
	for _, name := range names {
		sum.Write([]byte(name + "\x00" + rhoAssetFiles[name] + "\x00"))
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "rho", "assets", module+"-"+hex.EncodeToString(sum.Sum(nil))[:16])
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err == nil {
		return dir
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return ""
	}
	staging, err := os.MkdirTemp(filepath.Dir(dir), module+"-")
	if err != nil {
		return ""
	}
	for _, name := range names {
		path := filepath.Join(staging, filepath.FromSlash(strings.TrimPrefix(name, prefix)))
		if os.MkdirAll(filepath.Dir(path), 0o700) != nil || os.WriteFile(path, []byte(rhoAssetFiles[name]), 0o600) != nil {
			os.RemoveAll(staging)
			return ""
		}
	}
	if os.WriteFile(filepath.Join(staging, ".complete"), nil, 0o600) != nil {
		os.RemoveAll(staging)
		return ""
	}
	if err := os.Rename(staging, dir); err != nil {
		// Another run of the program wrote the same content first.
		os.RemoveAll(staging)
		if _, statErr := os.Stat(filepath.Join(dir, ".complete")); statErr != nil {
			return ""
		}
	}
	return dir
}
`
