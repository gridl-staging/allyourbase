// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/sites_handler.go.
package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/sites"
)

// siteManager is the interface for admin site and deploy operations.
// sites.Service satisfies this interface.
type siteManager interface {
	CreateSite(ctx context.Context, name, slug string, spaMode bool, customDomainID *string) (*sites.Site, error)
	GetSite(ctx context.Context, id string) (*sites.Site, error)
	ListSites(ctx context.Context, page, perPage int) (*sites.SiteListResult, error)
	UpdateSite(ctx context.Context, id string, name *string, spaMode *bool, customDomainID *string, clearCustomDomain bool) (*sites.Site, error)
	DeleteSite(ctx context.Context, id string) error

	CreateDeploy(ctx context.Context, siteID string) (*sites.Deploy, error)
	GetDeploy(ctx context.Context, siteID, deployID string) (*sites.Deploy, error)
	EnsureDeployUploading(ctx context.Context, siteID, deployID string) error
	RecordDeployFileUpload(ctx context.Context, siteID, deployID string, fileSize int64) (*sites.Deploy, error)
	ListDeploys(ctx context.Context, siteID string, page, perPage int) (*sites.DeployListResult, error)
	PromoteDeploy(ctx context.Context, siteID, deployID string) (*sites.Deploy, error)
	FailDeploy(ctx context.Context, siteID, deployID, errorMsg string) (*sites.Deploy, error)
	RollbackDeploy(ctx context.Context, siteID string) (*sites.Deploy, error)
}

// --- request bodies ---

type createSiteRequest struct {
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	SPAMode        bool    `json:"spaMode"`
	CustomDomainID *string `json:"customDomainId,omitempty"`
}

type updateSiteRequest struct {
	Name              *string `json:"name,omitempty"`
	SPAMode           *bool   `json:"spaMode,omitempty"`
	CustomDomainID    *string `json:"customDomainId,omitempty"`
	ClearCustomDomain bool    `json:"clearCustomDomain,omitempty"`
}

type failDeployRequest struct {
	ErrorMessage string `json:"errorMessage"`
}

func isBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}

// --- ID extraction helpers ---

func extractSiteID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := parseUUIDParamWithLabel(w, r, "siteId", "site id")
	if !ok {
		return "", false
	}
	return id.String(), true
}

func extractDeployID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := parseUUIDParamWithLabel(w, r, "deployId", "deploy id")
	if !ok {
		return "", false
	}
	return id.String(), true
}

// --- site handlers ---

func handleAdminListSites(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		result, err := svc.ListSites(r.Context(), page, perPage)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list sites")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

// TODO: Document handleAdminCreateSite.
func handleAdminCreateSite(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSiteRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if isBlank(req.Name) {
			httputil.WriteError(w, http.StatusBadRequest, "site name is required")
			return
		}
		if isBlank(req.Slug) {
			httputil.WriteError(w, http.StatusBadRequest, "site slug is required")
			return
		}

		site, err := svc.CreateSite(r.Context(), req.Name, req.Slug, req.SPAMode, req.CustomDomainID)
		if err != nil {
			if errors.Is(err, sites.ErrSiteSlugTaken) {
				httputil.WriteError(w, http.StatusConflict, "site slug already taken")
				return
			}
			if errors.Is(err, sites.ErrSiteCustomDomainTaken) {
				httputil.WriteError(w, http.StatusConflict, "custom domain is already attached to another site")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create site")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, site)
	}
}

// TODO: Document handleAdminGetSite.
func handleAdminGetSite(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := extractSiteID(w, r)
		if !ok {
			return
		}

		site, err := svc.GetSite(r.Context(), id)
		if err != nil {
			if errors.Is(err, sites.ErrSiteNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "site not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get site")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, site)
	}
}

// TODO: Document handleAdminUpdateSite.
func handleAdminUpdateSite(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := extractSiteID(w, r)
		if !ok {
			return
		}

		var req updateSiteRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Name != nil && isBlank(*req.Name) {
			httputil.WriteError(w, http.StatusBadRequest, "site name is required")
			return
		}

		site, err := svc.UpdateSite(r.Context(), id, req.Name, req.SPAMode, req.CustomDomainID, req.ClearCustomDomain)
		if err != nil {
			if errors.Is(err, sites.ErrSiteNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "site not found")
				return
			}
			if errors.Is(err, sites.ErrSiteCustomDomainTaken) {
				httputil.WriteError(w, http.StatusConflict, "custom domain is already attached to another site")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update site")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, site)
	}
}

// TODO: Document handleAdminDeleteSite.
func handleAdminDeleteSite(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := extractSiteID(w, r)
		if !ok {
			return
		}

		err := svc.DeleteSite(r.Context(), id)
		if err != nil {
			if errors.Is(err, sites.ErrSiteNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "site not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete site")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- deploy handlers ---

// TODO: Document handleAdminListDeploys.
func handleAdminListDeploys(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := extractSiteID(w, r)
		if !ok {
			return
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		result, err := svc.ListDeploys(r.Context(), siteID, page, perPage)
		if err != nil {
			if errors.Is(err, sites.ErrSiteNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "site not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list deploys")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

// TODO: Document handleAdminCreateDeploy.
func handleAdminCreateDeploy(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := extractSiteID(w, r)
		if !ok {
			return
		}

		deploy, err := svc.CreateDeploy(r.Context(), siteID)
		if err != nil {
			if errors.Is(err, sites.ErrSiteNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "site not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create deploy")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, deploy)
	}
}

// TODO: Document handleAdminGetDeploy.
func handleAdminGetDeploy(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := extractSiteID(w, r)
		if !ok {
			return
		}
		deployID, ok := extractDeployID(w, r)
		if !ok {
			return
		}

		deploy, err := svc.GetDeploy(r.Context(), siteID, deployID)
		if err != nil {
			if errors.Is(err, sites.ErrDeployNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "deploy not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get deploy")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, deploy)
	}
}

// TODO: Document handleAdminPromoteDeploy.
func handleAdminPromoteDeploy(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := extractSiteID(w, r)
		if !ok {
			return
		}
		deployID, ok := extractDeployID(w, r)
		if !ok {
			return
		}

		deploy, err := svc.PromoteDeploy(r.Context(), siteID, deployID)
		if err != nil {
			if errors.Is(err, sites.ErrDeployNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "deploy not found")
				return
			}
			if errors.Is(err, sites.ErrInvalidTransition) {
				httputil.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to promote deploy")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, deploy)
	}
}

// TODO: Document handleAdminFailDeploy.
func handleAdminFailDeploy(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := extractSiteID(w, r)
		if !ok {
			return
		}
		deployID, ok := extractDeployID(w, r)
		if !ok {
			return
		}

		var req failDeployRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		deploy, err := svc.FailDeploy(r.Context(), siteID, deployID, req.ErrorMessage)
		if err != nil {
			if errors.Is(err, sites.ErrDeployNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "deploy not found")
				return
			}
			if errors.Is(err, sites.ErrInvalidTransition) {
				httputil.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to fail deploy")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, deploy)
	}
}

// TODO: Document handleAdminRollbackDeploy.
func handleAdminRollbackDeploy(svc siteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := extractSiteID(w, r)
		if !ok {
			return
		}

		deploy, err := svc.RollbackDeploy(r.Context(), siteID)
		if err != nil {
			if errors.Is(err, sites.ErrNoLiveDeploy) {
				httputil.WriteError(w, http.StatusConflict, "no superseded deploy to rollback to")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to rollback deploy")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, deploy)
	}
}
