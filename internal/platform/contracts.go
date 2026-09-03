package platform

import "context"

type PackageAdapter interface {
	Inventory(context.Context) (any, error)
	Capture(context.Context, string) (any, error)
	Install(context.Context, any, func(current string, completed, total int)) error
}

type SecurityMetadataAdapter interface {
	Reconcile(context.Context, []string) error
}
type LandingAdapter interface {
	Qualify(context.Context, string) error
}
type IntegrationAdapter interface {
	Install(context.Context) error
	Uninstall(context.Context) error
}
