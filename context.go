/*
Package context is a proof of concept implementation of **scoped context**,
proposed in (this blog post) https://posener.github.io/goroutine-scoped-context.

This library should not be used for production code.

Usage

The context package should be imported from `github.com/posener/context`.

	 import (
	-   "context"
	+   "github.com/posener/context"
	 )

Since this implementation does not involve changes to the runtime,
the goroutine context must be initialized.

	 func main() {
	+	context.Init()
	 	// Go code goes here.
	 }

Functions should not anymore receive the context in the first argument.
They should get it from the goroutine scope.

	-func foo(ctx context.Context) {
	+func foo() {
	+	ctx := context.Get()
	 	// Use context.
	 }

Applying context to a scope:

	unset := context.Set(ctx)
	// ctx is applied until unset is called, or a deeper `Set` call.
	unset()

Or:

	defer context.Set(ctx)()
	// ctx is applied until the end of the function or a deeper `Set` call.

Invoking goroutines should be done with `context.Go` or `context.GoCtx`

Running a new goroutine with the current stored context:

	-go foo()
	+context.Go(foo)

More complected functions:

	-go foo(1, "hello")
	+context.Go(func() { foo(1, "hello") })

Running a goroutine with a new context:

	// `ctx` is the context that we want to have in the invoked goroutine
	context.GoCtx(ctx, foo)

`context.TODO` should not be used anymore:

	-f(context.TODO())
	+f(context.Get())
*/
package context

import (
	stdctx "context"
	"sync"

	"github.com/noke-inc/lib_context/runtime"
)

type (
	Context    = stdctx.Context
	CancelFunc = stdctx.CancelFunc
)

var (
	WithCancel   = stdctx.WithCancel
	WithTimeout  = stdctx.WithTimeout
	WithDeadline = stdctx.WithDeadline
	WithValue    = stdctx.WithValue

	Background = stdctx.Background

	DeadlineExceeded = stdctx.DeadlineExceeded
	Canceled         = stdctx.Canceled
)

var (
	// storage is used instead of goroutine local storage to
	// store goroutine(ID) to Context mapping.
	storage map[uint64][]Context
	// mutex for locking the storage map.
	mu sync.RWMutex
)

func init() {
	storage = make(map[uint64][]Context)
	Init()
}

// peek simulates fetching of context from goroutine local storage
// It gets the context from `storage` map according to the current
// goroutine ID.
// If the goroutine ID is not in the map, it panic. This case
// may occur when a user did not use the `context.Go` or `context.GoCtx`
// to invoke a goroutine.
// Note: real goroutine local storage won't need the implemented locking
// exists in this implementation, since the storage won't be accessible from
// different goroutines.
func peek() Context {
	id := runtime.GID()
	mu.RLock()
	defer mu.RUnlock()
	stack := storage[id]
	if stack == nil {
		panic("goroutine ran without using context.Go or context.GoCtx")
	}
	return stack[len(stack)-1]
}

// push simulates storing of context in the goroutine local storage.
// It gets the context to push to the context stack, and returns a pop function.
// Note: real goroutine local storage won't need the implemented locking
// exists in this implementation, since the storage won't be accessible from
// different goroutines.
func push(ctx Context) func() {
	id := runtime.GID()
	mu.Lock()
	defer mu.Unlock()
	storage[id] = append(storage[id], ctx)
	size := len(storage[id])
	return func() { pop(id, size) }
}

// pop simulates removal of a context from the thread local storage.
// If the stack is emptied, it will be removed from the storage map.
// Note: real goroutine local storage won't need the implemented locking
// exists in this implementation, since the storage won't be accessible from
// different goroutines.
func pop(id uint64, stackSize int) {
	mu.Lock()
	defer mu.Unlock()
	if len(storage[id]) != stackSize {
		if len(storage[id]) < stackSize {
			panic("multiple call for unset")
		}
		panic("there are contexts that should be unset before")
	}
	storage[id] = storage[id][:len(storage[id])-1]
	// Remove the stack from the map if it was emptied
	if len(storage[id]) == 0 {
		delete(storage, id)
	}
}

// Init creates the first background context in a program.
// it should be called once, in the beginning of the main
// function or in init() function.
// It returns the created context.
// All following goroutine invocations should be replaced
// by context.Go or context.GoCtx.
//
// Note:
// 		This function won't be needed in the real implementation.
func Init() Context {
	ctx := Background()
	push(ctx)
	return ctx
}

// Get gets the context of the current goroutine
// It may panic if the current go routine did not ran with
// context.Go or context.GoCtx.
//
// Note:
// 		This function won't panic in the real implementation.
func Get() Context {
	return peek()
}

// Set creates a context scope.
// It returns an "unset" function that should invoked at the
// end of this context scope. In any case, it must be invoked,
// exactly once, and in the right order.
func Set(ctx Context) func() {
	return push(ctx)
}

// Go invokes f in a new goroutine and takes care of propagating
// the current context to the created goroutine.
// It may panic if the current goroutine was not invoked with
// context.Go or context.GoCtx.
//
// Note:
// 		In the real implementation, this should be the behavior
// 		of the `go` keyword. It will also won't panic.
func Go(f func()) {
	GoCtx(peek(), f)
}

// GoCtx invokes f in a new goroutine with the given context.
//
// Note:
// 		In the real implementation, accepting the context argument
//		should be incorporated into the behavior of the `go` keyword.
func GoCtx(ctx Context, f func()) {
	go func() {
		pop := push(ctx)
		defer pop()
		f()
	}()
}
