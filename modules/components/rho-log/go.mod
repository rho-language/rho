module github.com/rho/rho-log

go 1.23

// The record format is defined once, in the formatter component, so a device
// writing to a serial console and a server writing to stdout cannot drift
// apart. Which checkout that is, the workspace says.
require github.com/rho/rho-fmt v0.0.0
