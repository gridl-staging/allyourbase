package matview

import "context"

// Admin combines Store CRUD and Service refresh operations for admin API use.
type Admin struct {
	store   *Store
	service *Service
}

// NewAdmin creates a combined admin facade.
func NewAdmin(store *Store, service *Service) *Admin {
	return &Admin{store: store, service: service}
}

func (a *Admin) List(ctx context.Context) ([]Registration, error) {
	return a.store.List(ctx)
}

func (a *Admin) Get(ctx context.Context, id string) (*Registration, error) {
	return a.store.Get(ctx, id)
}

func (a *Admin) Register(ctx context.Context, schemaName, viewName string, mode RefreshMode) (*Registration, error) {
	return a.store.Register(ctx, schemaName, viewName, mode)
}

func (a *Admin) Update(ctx context.Context, id string, mode RefreshMode) (*Registration, error) {
	return a.store.Update(ctx, id, mode)
}

func (a *Admin) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

func (a *Admin) RefreshNow(ctx context.Context, id string) (*RefreshResult, error) {
	return a.service.RefreshNow(ctx, id)
}
