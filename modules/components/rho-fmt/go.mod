module github.com/rho/rho-fmt

go 1.18

// The formatter needs the Rho front end. A component is versioned
// independently, so it depends on the compiler as a module rather than being
// compiled into it. The replace makes an in-tree build use this checkout;
// a published component pins a released version instead.
require rho v0.0.0

replace rho => ../../..
