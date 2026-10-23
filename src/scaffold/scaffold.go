//
// @file scaffold.go
// @brief Write a new Rho application: what a person starts from.
//
// A new project is four files and a decision about what it builds for. The
// files are not templates to be copied from somewhere else on disk — a copy
// would be a second description of a project that drifts from the one the
// toolchain enforces — so they are written here, from the same manifest shape
// `deps.LoadManifest` reads and the same descriptor the build uses.
//
// The same function is what `rho init`, `rho pkg init` and the keeper call. A
// generator reached from three places that each had their own copy would be
// three generators, and the first time one of them wrote a manifest the others
// did not accept, the disagreement would be the only thing a person saw.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
	"rho/src/yaml"
)

// Options is what to write and where.
type Options struct {
	// Dir is the directory to create the application in. It is created when it
	// does not exist; a directory that already holds a module.yml is refused
	// rather than written over.
	Dir string
	// Name is the module name. Empty means the directory's base name, made into
	// a name a module may have.
	Name string
	// Description is the manifest's one line about what the program is.
	Description string
	// Language is the build language: "go" (the default) or "c".
	Language string
	// Targets are the build targets to declare. Empty means a single target for
	// the language, which is a command-line program.
	Targets []string
}

// Result is what was written, so a caller can say so without guessing.
type Result struct {
	// Dir is the directory, absolute.
	Dir string
	// Name is the module name that was written.
	Name string
	// Files are the paths written, relative to Dir, in the order they were.
	Files []string
}

// validName makes a module name out of a directory name: lower case, and only
// the characters a name may hold. It starts with a letter, because a name that
// started with a digit would be a source module a reader could not tell from a
// number.
func validName(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ' || r == '.':
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return "app"
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "app_" + name
	}
	return name
}

// Application writes a minimal Rho application and returns what it wrote. The
// result builds with `rho build` and runs with `rho run`, and it depends on
// nothing, so it is a starting point rather than an example of something else.
func Application(opts Options) (Result, error) {
	if strings.TrimSpace(opts.Dir) == "" {
		return Result{}, fmt.Errorf("a directory to create the application in is required")
	}
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Result{}, fmt.Errorf("cannot resolve %s: %w", opts.Dir, err)
	}
	// The directory is where the project is created, and it must not climb
	// out of the place it was given.
	if strings.Contains(dir, "..") {
		return Result{}, fmt.Errorf("invalid directory path")
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = validName(filepath.Base(dir))
	} else {
		name = validName(name)
	}

	manifestPath := filepath.Join(dir, "module.yml")
	if _, err := os.Stat(manifestPath); err == nil {
		return Result{}, fmt.Errorf("%s already holds a module.yml; choose another directory", dir)
	}

	language := strings.TrimSpace(opts.Language)
	if language == "" {
		language = "go"
	}
	targets := opts.Targets
	if len(targets) == 0 {
		targets = []string{language}
	}

	srcDir := filepath.Join(dir, "src")
	// Validate the source directory path before creating it.
	cleanSrcDir := filepath.Clean(srcDir)
	if filepath.IsAbs(cleanSrcDir) && !strings.Contains(cleanSrcDir, "..") {
		srcDir = cleanSrcDir
	}
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("cannot create %s: %w", srcDir, err)
	}

	description := strings.TrimSpace(opts.Description)
	if description == "" {
		description = "A Rho application."
	}

	files := []struct {
		path    string
		content string
	}{
		{"module.yml", manifest(name, description, language, targets)},
		{filepath.Join("src", "main.rho"), program(name)},
		{"README.md", readme(name, language, targets)},
		{".gitignore", gitignore()},
	}

	written := make([]string, 0, len(files))
	for _, file := range files {
		full := filepath.Join(dir, file.path)
		if err := os.WriteFile(full, []byte(file.content), 0o644); err != nil {
			return Result{}, fmt.Errorf("cannot write %s: %w", file.path, err)
		}
		written = append(written, file.path)
	}

	return Result{Dir: dir, Name: name, Files: written}, nil
}

// manifest is the module.yml a new application starts with. Its shape is what
// `deps.LoadManifest` reads, so a generated project is a project the toolchain
// already knows rather than one a later version has to be taught.
func manifest(name, description, language string, targets []string) string {
	var b strings.Builder
	b.WriteString("#\n# @file module.yml\n# @brief ")
	b.WriteString(name)
	b.WriteString(" — a Rho application.\n#\n\n")
	b.WriteString("module:\n")
	fmt.Fprintf(&b, "  name: %s\n", name)
	b.WriteString("  type: rho\n")
	b.WriteString("  version: 0.1.0\n")
	fmt.Fprintf(&b, "  description: %s\n", description)
	b.WriteString("\nmain: src/main.rho\n\n")
	b.WriteString("build:\n")
	fmt.Fprintf(&b, "  language: %s\n", language)
	b.WriteString("  targets:\n")
	for _, target := range targets {
		fmt.Fprintf(&b, "    - %s\n", target)
	}
	return b.String()
}

// program is the entry point. It prints one line and returns, which is the
// smallest thing that compiles to every backend and runs everywhere.
func program(name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\n", name)
	b.WriteString("// main is where the program starts. Every backend \u2014 the native one and\n")
	b.WriteString("// the generated service \u2014 reaches it the same way, so this is the one place\n")
	b.WriteString("// a program has to be itself.\n")
	b.WriteString("fn main() void {\n")
	fmt.Fprintf(&b, "    print(\"Hello from %s!\\n\")\n", name)
	b.WriteString("}\n")
	return b.String()
}

func readme(name, language string, targets []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	b.WriteString("A Rho application.\n\n")
	b.WriteString("```sh\nrho build .   # generate and build\nrho run .     # build, then run\n```\n\n")
	fmt.Fprintf(&b, "The program is `src/main.rho`. The generated projects are written to\n")
	fmt.Fprintf(&b, "`build/generated/<target>/` for the targets declared in `module.yml`")
	fmt.Fprintf(&b, " (%s).\n", strings.Join(targets, ", "))
	return b.String()
}

func gitignore() string {
	return "/build/\n"
}

// Request is one `rho init`: where, and with what.
type Request struct {
	// Project is the directory whose module.yml may declare generators. Empty
	// means the working directory.
	Project string
	// Dir is where the project is written.
	Dir string
	// Name is the name of what is generated.
	Name string
	// Generator chooses among several; empty means the only one.
	Generator string
	// Category and Description are the variables a driver generator takes, and
	// Set carries any others the generator declares.
	Category    string
	Description string
	Set         map[string]string
	// Language and Targets are for the Rho application case.
	Language string
	Targets  []string
}

func validateInitDir(dir string) (string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "", fmt.Errorf("a directory to create the project in is required")
	}
	if strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("invalid directory path")
	}
	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("directory path must be absolute")
	}
	return cleaned, nil
}

// Init writes what the project offers: a generator's outputs when the project
// declares `generate:`, and a Rho application otherwise. It is the whole
// decision, so `rho init` and the keeper write the same thing from the same
// place rather than each deciding for itself.
func Init(req Request) ([]Result, string, error) {
	project := strings.TrimSpace(req.Project)
 	if project == "" {
		project = "."
	} else {
 		// Project is expected to identify a single workspace component (module
 		// directory), not an arbitrary path. Reject separators and traversal
 		// markers before any path operations.
 		if strings.Contains(project, "/") || strings.Contains(project, "\\") || strings.Contains(project, "..") {
 			return nil, "", errors.New("project path must be a single relative path component within the workspace")
 		}
	}
	project = filepath.Clean(project)
	if filepath.IsAbs(project) || project == "." || project == ".." || strings.HasPrefix(project, ".."+string(filepath.Separator)) {
 		return nil, "", errors.New("project path must be a single relative path component within the workspace")
	}
	if strings.TrimSpace(req.Dir) == "" {
		return nil, "", fmt.Errorf("a directory to create the project in is required")
	}

	generators, err := Generators(project)
	if err != nil {
		return nil, "", err
	}
	if len(generators) == 0 {
		result, err := Application(Options{
			Dir:         req.Dir,
			Name:        req.Name,
			Description: req.Description,
			Language:    req.Language,
			Targets:     req.Targets,
		})
		return []Result{result}, "", err
	}

	name := strings.TrimSpace(req.Generator)
	if name == "" {
		if len(generators) == 1 {
			name = generators[0].Name
		} else {
			return nil, "", fmt.Errorf("%s declares several generators; name one", project)
		}
	}
	generator, ok, err := Find(project, name)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("%s declares no generator named %q", project, name)
	}

	values := map[string]string{
		"name":        req.Name,
		"category":    req.Category,
		"description": req.Description,
	}
	for key, value := range req.Set {
		values[key] = value
	}
	if strings.TrimSpace(values["description"]) == "" {
		values["description"] = generator.Description
	}

	missing := []string{}
	for _, variable := range generator.Variables {
		if strings.TrimSpace(values[variable.Name]) == "" {
			missing = append(missing, variable.Name)
		}
	}
	if len(missing) > 0 {
		return nil, name, fmt.Errorf("generator %q needs %s", name, strings.Join(missing, ", "))
	}

	results, err := Render(project, generator, req.Dir, values)
	return results, name, err
}

/* -------------------------------------------------------------------------- */
/*  Generators a module declares                                               */
/* -------------------------------------------------------------------------- */

// Variable is one thing a generator asks for: a driver's category, its name.
type Variable struct {
	// Name is the placeholder it fills, as `{{name}}`.
	Name string
	// Description is what the question means, shown when the generator is
	// listed.
	Description string
}

// Output is one directory a generator writes. A generator that produces a
// driver and an example that uses it has two outputs, and they are written
// together, because the example is not useful without the driver it names.
type Output struct {
	// Path is where the directory goes, relative to the destination, with the
	// generator's variables substituted: `drivers/{{category}}/{{name}}`.
	Path string
	// Template is the directory of files, relative to the module root.
	Template string
}

// Generator is a kind of project a module knows how to start: what it is, what
// it asks for, and what it writes.
type Generator struct {
	Name        string
	Description string
	Variables   []Variable
	Outputs     []Output
}

// Generators reads what a project offers to generate, from `generate:` in its
// module.yml: the definitions themselves, or a path to a YAML file holding
// them. A project that declares none returns nothing, which is how `rho init`
// knows to write a Rho application instead.
func Generators(projectRoot string) ([]Generator, error) {
	manifestPath := deps.ManifestPath(projectRoot)
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, nil
	}
	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", manifestPath, err)
	}

	// A manifest with no `generate:` at all reads as the zero value, whose kind
	// is the same number as a string; only a non-empty string is a file name.
	node := manifest.Generate
	if node.Kind == yaml.KindString && strings.TrimSpace(node.Str) != "" {
		reference := strings.TrimSpace(node.Str)
		content, err := os.ReadFile(filepath.Join(projectRoot, reference))
		if err != nil {
			return nil, fmt.Errorf("cannot read the generate file %s: %w", reference, err)
		}
		root, err := yaml.Load(string(content))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", reference, err)
		}
		node = root.Map["generators"]
		if node.Kind == 0 {
			node = root
		}
	}
	if node.Kind != yaml.KindList {
		return nil, nil
	}

	generators := make([]Generator, 0, len(node.List))
	seen := map[string]bool{}
	for _, item := range node.List {
		if item.Kind != yaml.KindMap {
			return nil, fmt.Errorf("a generator entry must be a map")
		}
		generator := Generator{
			Name:        strings.TrimSpace(item.Map["name"].Str),
			Description: strings.TrimSpace(item.Map["description"].Str),
		}
		if generator.Name == "" {
			return nil, fmt.Errorf("a generator is missing a name")
		}
		if seen[generator.Name] {
			return nil, fmt.Errorf("generator %q is declared more than once", generator.Name)
		}
		seen[generator.Name] = true

		for _, variable := range item.Map["variables"].List {
			if variable.Kind != yaml.KindMap {
				return nil, fmt.Errorf("generator %q has a variable that is not a map", generator.Name)
			}
			generator.Variables = append(generator.Variables, Variable{
				Name:        strings.TrimSpace(variable.Map["name"].Str),
				Description: strings.TrimSpace(variable.Map["description"].Str),
			})
		}

		for _, output := range item.Map["outputs"].List {
			if output.Kind != yaml.KindMap {
				return nil, fmt.Errorf("generator %q has an output that is not a map", generator.Name)
			}
			entry := Output{
				Path:     strings.TrimSpace(output.Map["path"].Str),
				Template: strings.TrimSpace(output.Map["template"].Str),
			}
			if entry.Path == "" || entry.Template == "" {
				return nil, fmt.Errorf("generator %q has an output without a path and template", generator.Name)
			}
			generator.Outputs = append(generator.Outputs, entry)
		}
		if len(generator.Outputs) == 0 {
			return nil, fmt.Errorf("generator %q declares no outputs", generator.Name)
		}
		generators = append(generators, generator)
	}
	return generators, nil
}

// Find returns the named generator, or false when the project declares none of
// that name.
func Find(projectRoot, name string) (Generator, bool, error) {
	generators, err := Generators(projectRoot)
	if err != nil {
		return Generator{}, false, err
	}
	for _, generator := range generators {
		if generator.Name == name {
			return generator, true, nil
		}
	}
	return Generator{}, false, nil
}

// Render writes every output of a generator under a destination root. Each
// output's files and directories are named and filled with the same
// substitutions, so a template says `{{name}}` once and the file named after
// the driver and the code inside it agree.
func Render(projectRoot string, generator Generator, destRoot string, values map[string]string) ([]Result, error) {
	if strings.TrimSpace(destRoot) == "" {
		return nil, fmt.Errorf("a directory to create the project in is required")
	}
	root, err := filepath.Abs(destRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve %s: %w", destRoot, err)
	}

	filled := map[string]string{}
	for key, value := range values {
		filled[key] = value
	}
	if strings.TrimSpace(filled["name"]) == "" {
		filled["name"] = validName(filepath.Base(root))
	} else {
		filled["name"] = validName(filled["name"])
	}

	results := make([]Result, 0, len(generator.Outputs))
	for _, output := range generator.Outputs {
		dir := filepath.Join(root, filepath.FromSlash(substitute(output.Path, filled)))
		templateRoot := filepath.Join(projectRoot, output.Template)
		if info, err := os.Stat(templateRoot); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("generator %q: template %s is not a directory", generator.Name, output.Template)
		}

		written, err := renderTree(templateRoot, dir, filled)
		if err != nil {
			return nil, fmt.Errorf("generator %q: %w", generator.Name, err)
		}
		results = append(results, Result{Dir: dir, Name: filled["name"], Files: written})
	}
	return results, nil
}

// renderTree copies a template directory into a destination, substituting the
// values in every file and directory name and in every file's contents.
func isWithinBase(baseAbs, candidate string) (bool, error) {
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false, err
	}
	relToBase, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return false, err
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func renderTree(templateRoot, destDir string, values map[string]string) ([]string, error) {
	written := []string{}
	err := filepath.WalkDir(templateRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(templateRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Substitute template values in the path. Each value is sanitized
		// to a single path component to prevent traversal.
		for key, value := range values {
			safe := strings.TrimSpace(value)
				safe = strings.ReplaceAll(safe, "\\", "/")
			safe = strings.ReplaceAll(safe, "..", "")
			safe = filepath.Base(safe)
			for _, spelling := range []string{"{{" + key + "}}", "{{ " + key + " }}"} {
				rel = strings.ReplaceAll(rel, spelling, safe)
			}
		}
		// Validate the resulting path.
		cleanRel := filepath.Clean(rel)
		if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid output path %q", rel)
		}
		rel = cleanRel

		target := filepath.Join(destDir, rel)


		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		safeContent := substitute(string(content), values)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(safeContent), 0o646); err != nil {
			return err
		}
		written = append(written, filepath.Join(filepath.Base(destDir), rel))
		return nil
	})
	return written, err
}

// substitute replaces `{{key}}` with its value, allowing spaces inside the
// braces. A placeholder with no value is left as it was written, so a template
// that wants literal braces still gets them and a misspelled key is visible in
// the output rather than silently blank.
func substitute(text string, values map[string]string) string {
	for key, value := range values {
		// Keep substitutions safe when placeholders are used in output paths:
		// normalize to a single path component and drop traversal markers.
		safe := strings.TrimSpace(value)
				safe = strings.ReplaceAll(safe, "\\", "/")
		safe = strings.ReplaceAll(safe, "..", "")
		safe = filepath.Base(safe)
		for _, spelling := range []string{"{{" + key + "}}", "{{ " + key + " }}"} {
			text = strings.ReplaceAll(text, spelling, safe)
		}
	}
	return text
}
