package ctx

import "context"

type handlerKey struct{}

// AppendID appends id to context
func AppendID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, handlerKey{}, id)
}

// ID returns context id
func ID(ctx context.Context) string {
	id, _ := ctx.Value(handlerKey{}).(string)
	return id
}
