module github.com/rho/rho-sql-sqlite

go 1.23

require (
	github.com/rho/rho-sql v0.0.0
	modernc.org/sqlite v1.28.0
)

require rho v0.0.0 // indirect

// The engine is a plugin: it is dispatched to rather than compiled into a
// service, and it is reached over the same protocol every plugin speaks.
//
// The driver is modernc's, which is sqlite compiled to Go rather than a C
// library behind cgo. An engine plugin that needed a C toolchain to build would
// be an engine that half the machines could not load, and the whole point of
// making the engine a plugin is that it is portable to wherever the service is.
// The version is pinned rather than floating: a driver that needs a newer Go
// than a component is built with cannot be loaded by a toolchain that predates
// it, and the failure would be at the first statement rather than at the build.
require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/mod v0.3.0 // indirect
	golang.org/x/sys v0.9.0 // indirect
	golang.org/x/tools v0.0.0-20201124115921-2c860bdd6e78 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	lukechampine.com/uint128 v1.2.0 // indirect
	modernc.org/cc/v3 v3.40.0 // indirect
	modernc.org/ccgo/v3 v3.16.13 // indirect
	modernc.org/libc v1.29.0 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.7.2 // indirect
	modernc.org/opt v0.1.3 // indirect
	modernc.org/strutil v1.1.3 // indirect
	modernc.org/token v1.0.1 // indirect
)
