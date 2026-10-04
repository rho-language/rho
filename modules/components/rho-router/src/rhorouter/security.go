//
// @file security.go
// @brief How a caller proves who it is, as the service declares it.
//
// A route says where a request goes and what it carries. It does not say what a
// caller has to prove, because nothing in a Rho handler says that: the check is
// a call in a body, and a call in a body can be conditional. A deployment that
// closes token issuance and one that leaves it open serve the same source.
//
// So a service states it, once, in the descriptor of the target that serves it:
//
//	- id: go-server
//	  emitter: go
//	  kind: server
//	  api:
//	    security: bearer
//	    security_description: "A publish token, obtained from POST /auth/token."
//	    public_paths: "/healthz, /"
//
// That is a fact about the deployment rather than about one handler, which is
// why it is declared rather than guessed at. A document that inferred a
// requirement from a call inside a conditional would be wrong about half the
// services it described, and wrong in the direction that makes a client fail.
//
// The pipeline is the other half. A target that wraps its router in the jwt
// wrapper requires a token on every route it does not exempt, whether or not
// any handler checks one — the wrapper refuses the request before a handler is
// reached. So that is declared by composing the wrapper, and the exempt paths
// are the wrapper's own settings.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhorouter

import (
	"os"
	"path/filepath"
	"strings"

	"rho/src/yaml"
)

// DescriptorFileName is the per-project descriptor a target is declared in,
// which is where a service says how a caller proves who it is.
const DescriptorFileName = "project.yml"

// MiddlewareJWT is the name of the wrapper that requires a bearer token. It is
// the one middleware name this component has to know, because it is the one
// that changes what a client must send rather than only what the service does
// with what it was sent.
const MiddlewareJWT = "jwt"

// The ways a caller proves who it is that a descriptor may name.
const (
	// SecurityNone is a service that asks a caller for nothing. It is the
	// default, because a service that has not said otherwise has not said
	// anything, and a document that claimed a requirement nobody declared
	// would be a document that made a client send a credential it has no
	// reason to hold.
	SecurityNone = "none"
	// SecurityBearer is an HTTP bearer token in the Authorization header: the
	// OAuth 2 and JWT shape, and the one the jwt wrapper enforces.
	SecurityBearer = "bearer"
)

// Security is how a service asks a caller to prove who it is.
type Security struct {
	// Scheme is the kind of credential: none or bearer.
	Scheme string `json:"scheme"`
	// Name is what a document calls the scheme, which is what an operation's
	// requirement refers to.
	Name string `json:"name"`
	// Description says where a caller gets a credential and what it is good
	// for, which is the one thing a document cannot derive from anything.
	Description string `json:"description,omitempty"`
	// Required is true when the pipeline refuses a request without a
	// credential, rather than when a handler may ask for one.
	Required bool `json:"required,omitempty"`
	// PublicPaths are the paths served without a credential. An exact match,
	// because that is what the wrapper it comes from does.
	PublicPaths []string `json:"publicPaths,omitempty"`
}

// declared reports whether a service said anything about credentials.
func (s *Security) declared() bool {
	return s != nil && s.Scheme != "" && s.Scheme != SecurityNone
}

// Public reports whether a path is served without a credential, compared
// exactly, which is what the wrapper that enforces the requirement does.
func (s *Security) Public(path string) bool {
	if s == nil {
		return true
	}
	for _, exempt := range s.PublicPaths {
		if exempt == path {
			return true
		}
	}
	return false
}

// Describe fills in what a document says about a scheme, leaving a declared
// description alone.
func (s *Security) Describe() Security {
	if s == nil {
		return Security{Scheme: SecurityNone}
	}
	out := *s
	if out.Name == "" {
		out.Name = out.Scheme + "Auth"
	}
	if out.Description == "" {
		out.Description = defaultSecurityDescription(out.Scheme)
	}
	return out
}

// defaultSecurityDescription says how a caller gets a credential, for a service
// that declared a scheme without saying where one comes from.
func defaultSecurityDescription(scheme string) string {
	switch scheme {
	case SecurityBearer:
		return "A bearer token in the Authorization header."
	default:
		return ""
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading it from a descriptor                                                */
/* -------------------------------------------------------------------------- */

// apiSection is what a target says about the HTTP API it serves, over and above
// what its routes say.
//
// It is read as data rather than against the compiler's descriptor schema,
// because the two are answering different questions: the schema validates a
// document a build will use, and this asks four questions of one so that a
// description can be written from it. Reading it here rather than importing the
// generator keeps a component that reads sources from carrying the code
// generator to do it.
type apiSection struct {
	// Security is the scheme a service asks for, empty when it asked for
	// nothing.
	Security string
	// Description says where a credential comes from.
	Description string
	// PublicPaths are the paths served without one.
	PublicPaths []string
}

// readTargetAPI reads the api section of a project's server target.
//
// The first target of kind server is the one a document describes, because a
// project that serves an HTTP API has one, and a project that serves two is a
// project that needs to say which — which its own descriptor does by putting
// the one it documents first.
func readTargetAPI(root, wanted string) (apiSection, Security, bool, error) {
	section := apiSection{}
	fromPipeline := Security{}
	path := filepath.Join(root, DescriptorFileName)
	content, err := os.ReadFile(path)
	if err != nil {
		// A project with no descriptor is served by a target the backend
		// chose, and has said nothing about credentials. That is an answer,
		// not a failure.
		return section, fromPipeline, false, nil
	}
	document, err := yaml.Load(string(content))
	if err != nil {
		return section, fromPipeline, false, err
	}

	for _, target := range document.Map["targets"].List {
		if target.Kind != yaml.KindMap {
			continue
		}
		if kind := target.Map["kind"].Str; kind != "" && kind != "server" {
			continue
		}
		if wanted != "" && target.Map["id"].Str != wanted {
			continue
		}
		api := target.Map["api"]
		if api.Kind == yaml.KindMap {
			section.Security = strings.TrimSpace(api.Map["security"].Str)
			section.Description = strings.TrimSpace(api.Map["security_description"].Str)
			section.PublicPaths = splitList(api.Map["public_paths"].Str)
		}
		// The pipeline is the other half of the answer, and it is the half
		// that is enforced rather than requested: the jwt wrapper refuses a
		// request before any handler sees it.
		for _, middleware := range target.Map["middleware"].List {
			if middleware.Kind != yaml.KindMap || middleware.Map["name"].Str != MiddlewareJWT {
				continue
			}
			fromPipeline = Security{
				Scheme:      SecurityBearer,
				Required:    true,
				PublicPaths: splitList(middleware.Map["options"].Map["skip_paths"].Str),
			}
		}
		return section, fromPipeline, true, nil
	}
	return section, fromPipeline, false, nil
}

// splitList reads a comma-separated list, dropping the empty entries a trailing
// separator leaves behind.
func splitList(value string) []string {
	out := []string{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// resolveSecurity folds what a descriptor declared and what its pipeline
// enforces into the one answer a document carries.
//
// The declared scheme wins on what a credential is and where it comes from,
// because a project that wrote one down knows more than a wrapper's name does.
// The pipeline wins on whether it is required, because a wrapper that refuses
// a request is not a request to negotiate.
func resolveSecurity(section apiSection, fromPipeline Security) *Security {
	security := fromPipeline
	if section.Security != "" {
		security.Scheme = section.Security
	}
	if security.Scheme == "" || security.Scheme == SecurityNone {
		return nil
	}
	if section.Description != "" {
		security.Description = section.Description
	}
	if len(section.PublicPaths) > 0 {
		security.PublicPaths = section.PublicPaths
	}
	return &security
}
