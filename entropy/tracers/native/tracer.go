/*
Package native is a collection of tracers written in go.

In order to add a native tracer and have it compiled into the binary, a new
file needs to be added to this folder, containing an implementation of the
`eth.tracers.Tracer` interface.

Aside from implementing the tracer, it also needs to register itself, using the
`register` method -- and this needs to be done in the package initialization.

Example:

```golang
func init() {
	register("noopTracerNative", newNoopTracer)
}
```
*/
package native

import (
	"errors"
	"github.com/entropyio/go-entropy/entropy/tracers"
)

// init registers itself this packages as a lookup for tracers.
func init() {
	tracers.RegisterLookup(false, lookup)
}

// ctorFn is the constructor signature of a native tracer.
type ctorFn = func(*tracers.Context) tracers.Tracer

/*
ctors is a map of package-local tracer constructors.

We cannot be certain about the order of init-functions within a package,
The go spec (https://golang.org/ref/spec#Package_initialization) says

> To ensure reproducible initialization behavior, build systems
> are encouraged to present multiple files belonging to the same
> package in lexical file name order to a compiler.

Hence, we cannot make the map in init, but must make it upon first use.
*/
var ctors map[string]ctorFn

// register is used by native tracers to register their presence.
func register(name string, ctor ctorFn) {
	if ctors == nil {
		ctors = make(map[string]ctorFn)
	}
	ctors[name] = ctor
}

// lookup returns a tracer, if one can be matched to the given name.
func lookup(name string, ctx *tracers.Context) (tracers.Tracer, error) {
	if ctors == nil {
		ctors = make(map[string]ctorFn)
	}
	if ctor, ok := ctors[name]; ok {
		return ctor(ctx), nil
	}
	return nil, errors.New("no tracer found")
}
