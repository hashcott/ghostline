// Package client is the Linux GUI's side of the daemon: Service has
// exactly app.Service's methods, each forwarding over the control socket,
// so Wails binds it in place of the in-process service and the frontend
// does not change.
package client

//go:generate go run ../../../tools/genrpc -out service_gen.go
