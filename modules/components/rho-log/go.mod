module github.com/rho/rho-log

go 1.23

// The record format is defined once, in the formatter component, so a device
// writing to a serial console and a server writing to stdout cannot drift
// apart. The replace makes an in-tree build use this checkout; a published
// component pins a released version instead.
require github.com/rho/rho-fmt v0.0.0

replace github.com/rho/rho-fmt => ../rho-fmt
