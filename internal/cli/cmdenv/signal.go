package cmdenv

import "context"

// signalKey is the context key under which WithSignalContext stores the signal-only context.
type signalKey struct{}

// WithSignalContext returns ctx carrying signal, the context that only Ctrl-C or SIGTERM cancels (no --timeout
// deadline). cli.Run calls it before it applies the --timeout deadline.
func WithSignalContext(ctx, signal context.Context) context.Context {
	return context.WithValue(ctx, signalKey{}, signal)
}

// SignalContext returns the signal-only context stored by WithSignalContext, or ctx itself when there is none.
// The phases that v2.6.1 never bounded by --timeout use it: the `instance log --follow` loop (each poll still gets its
// own --timeout) and the `vcr debug` session after setup. Everything else runs under cmd.Context()'s deadline.
func SignalContext(ctx context.Context) context.Context {
	if signal, ok := ctx.Value(signalKey{}).(context.Context); ok {
		return signal
	}
	return ctx
}
