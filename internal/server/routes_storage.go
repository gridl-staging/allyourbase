package server

import (
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/go-chi/chi/v5"
)

// registerStorageRoutes mounts storage bucket admin CRUD, user object
// operations, signed URL creation, and resumable upload endpoints.
func (s *Server) registerStorageRoutes(r chi.Router) {
	if s.storageHandler == nil {
		return
	}
	// Storage routes accept multipart/form-data, mounted outside JSON content-type enforcement.
	r.Route("/storage", func(r chi.Router) {
		r.Route("/buckets", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Post("/", s.storageHandler.HandleBucketCreate)
			r.Get("/", s.storageHandler.HandleBucketList)
			r.Put("/{name}", s.storageHandler.HandleBucketUpdate)
			r.Delete("/{name}", s.storageHandler.HandleBucketDelete)
		})

		if s.authSvc != nil {
			// Read operations: auth optional (supports signed URLs).
			r.Group(func(r chi.Router) {
				r.Use(auth.OptionalAuth(s.authSvc))
				r.Get("/{bucket}", s.storageHandler.HandleList)
				r.Get("/{bucket}/*", s.storageHandler.HandleServe)
			})
			// Write operations: admin or user auth required.
			s.withTenantScopedAdminOrUserAuth(r, func(r chi.Router) {
				r.Post("/{bucket}", s.storageHandler.HandleUpload)
				r.Delete("/{bucket}/*", s.storageHandler.HandleDelete)
				r.Post("/{bucket}/{name}/sign", s.storageHandler.HandleSign)
			})
			r.Route("/upload/resumable", func(r chi.Router) {
				// TUS preflight must stay unauthenticated because browsers do not
				// send bearer tokens on OPTIONS requests.
				r.Options("/", s.storageHandler.HandleResumableOptions)
				s.withTenantScopedAdminOrUserAuth(r, func(r chi.Router) {
					r.Post("/", s.storageHandler.HandleResumableCreate)
					r.Head("/{id}", s.storageHandler.HandleResumableHead)
					r.Patch("/{id}", s.storageHandler.HandleResumablePatch)
				})
			})
		} else {
			r.Mount("/", s.storageHandler.Routes())
		}
	})
}
