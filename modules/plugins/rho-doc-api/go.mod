module github.com/rho/rho-doc-api

go 1.23

require (
	github.com/rho/rho-openapi v0.0.0
	github.com/rho/rho-router v0.0.0
	rho v0.0.0
)

// The plugin speaks the documentation generator's protocol, which is a pair of
// JSON documents rather than a library, so it needs nothing from rho-doc. What
// it does need is the reading: a page that lists a service's endpoints has to
// come from somewhere, and reading Rho sources a second time would be a second
// dialect of Rho — which is the problem rho-router exists to solve. So it
// depends on the same reading the service's own description is written from,
// and the page and the document cannot disagree.
replace (
	github.com/rho/rho-openapi => ../../components/rho-openapi
	github.com/rho/rho-router => ../../components/rho-router
	rho => ../../..
)
