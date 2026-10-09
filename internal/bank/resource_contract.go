package bank

import (
	"context"

	sdkSession "github.com/vasyza/sber-sdk/internal/session"
)

// BusinessRequester is the resource/client boundary. It performs one read or
// explicitly permitted mutation; resource APIs never own transport or auth.
// The sequence sender is valid only during its callback. Never retain it or
// reacquire Mutate from inside MutationSequence.
type BusinessRequester interface {
	PostRead(context.Context, string, map[string]any) (map[string]any, error)
	Mutate(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
	MutationSequence(context.Context, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error
	ExportSession() (sdkSession.SessionBundle, error)
	ExportCredentials() (sdkSession.SberCredentials, error)
	WarmUp(context.Context, bool) error
}
