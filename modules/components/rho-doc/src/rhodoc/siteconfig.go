//
// @file siteconfig.go
// @brief What a repository says about the site its documentation becomes.
//
// A module describes its pages in its manifest. A repository has no manifest of
// its own, so the shape of the site it documents — its title, the order and the
// names of its sections, the page a reader lands on, and where the API
// reference is read from — is written in `docs/site.yml`:
//
//	title: Rho
//	front: documentation/index     # the page index.html opens
//	api: std                       # the reference is read from std/ (none: no reference)
//	sections:
//	  - name: documentation
//	    title: Start
//	    pages: [index, getting-started, cli]
//	  - name: language
//
// A section the file does not name follows the named ones, alphabetically; a
// page a section does not list follows the listed ones, as before. A page is
// listed by its file name without the extension, or by the module it belongs
// to, which is how a section of components is put in order.
//
// @project Rho
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodoc

import (
	"os"
	"path/filepath"
	"strings"

	"rho/src/yaml"
)

// SiteConfig is a repository's description of its site.
type SiteConfig struct {
	Title    string
	Front    string
	API      string
	Sections []SectionSpec
}

// SectionSpec is one section a site names: where it goes, what it is called,
// and the order of its pages.
type SectionSpec struct {
	Name  string
	Title string
	Pages []string
}

// ReadSiteConfig reads `docs/site.yml` under root; a tree without one is
// described by nothing, and the site is built the way it always was.
func ReadSiteConfig(root string) (SiteConfig, error) {
	data, err := os.ReadFile(filepath.Join(root, "docs", "site.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return SiteConfig{}, nil
		}
		return SiteConfig{}, err
	}
	doc, err := yaml.Load(string(data))
	if err != nil {
		return SiteConfig{}, err
	}
	config := SiteConfig{
		Title: siteString(doc, "title"),
		Front: siteString(doc, "front"),
		API:   siteString(doc, "api"),
	}
	for _, item := range doc.Map["sections"].List {
		spec := SectionSpec{Name: siteString(item, "name"), Title: siteString(item, "title")}
		if item.Kind == yaml.KindString {
			spec.Name = yaml.Unquote(item.Str)
		}
		for _, page := range item.Map["pages"].List {
			if page.Kind == yaml.KindString {
				spec.Pages = append(spec.Pages, yaml.Unquote(page.Str))
			}
		}
		if spec.Name != "" {
			config.Sections = append(config.Sections, spec)
		}
	}
	return config, nil
}

func siteString(value yaml.Value, key string) string {
	if item, ok := value.Map[key]; ok && item.Kind == yaml.KindString {
		return strings.TrimSpace(yaml.Unquote(item.Str))
	}
	return ""
}

// sectionRank is where a section goes: its place in the list, or after every
// listed one.
func sectionRank(specs []SectionSpec, name string) int {
	for index, spec := range specs {
		if spec.Name == name {
			return index
		}
	}
	return len(specs)
}

// pageRank is where a page goes in its section: its place in the section's
// list, by file name or module, or after every listed one.
func pageRank(specs []SectionSpec, page *parsedPage) int {
	for _, spec := range specs {
		if spec.Name != page.section {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(page.document.Path), filepath.Ext(page.document.Path))
		for index, name := range spec.Pages {
			if name == stem || (page.module != "" && name == page.module) || name == page.document.Target {
				return index
			}
		}
		return len(spec.Pages)
	}
	return 0
}

// sectionLabel is what a section is called in the menu: the title the site
// gives it, or its name in words.
func (site *Site) sectionLabel(name string) string {
	for _, spec := range site.sectionSpecs {
		if spec.Name == name && spec.Title != "" {
			return spec.Title
		}
	}
	return humanise(name)
}
