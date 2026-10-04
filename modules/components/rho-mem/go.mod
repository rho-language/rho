module github.com/rho/rho-mem

go 1.23

// The component reads what a linker wrote: a `.map` file and the ELF beside it.
// Both are formats rather than libraries, so it depends on nothing from this
// toolchain except the manifest model — which is the one thing here that is not a
// format, and is shared with every other component so that a project's `mem:`
// block means the same thing to all of them.
//
// Everything it knows about a board comes from the artifacts and the project's
// own manifest. There is no list of memory regions here, and no per-vendor
// arithmetic, because a component that carried those would be wrong on the first
// board nobody had listed.
