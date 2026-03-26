package server

import (
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
)

type securityAdvisorReport struct {
	EvaluatedAt string            `json:"evaluatedAt"`
	Stale       bool              `json:"stale"`
	Findings    []securityFinding `json:"findings"`
}

type securityFinding struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
}

type performanceAdvisorReport struct {
	GeneratedAt string             `json:"generatedAt"`
	Stale       bool               `json:"stale"`
	Range       string             `json:"range"`
	Queries     []performanceQuery `json:"queries"`
}

type performanceQuery struct {
	Fingerprint     string   `json:"fingerprint"`
	NormalizedQuery string   `json:"normalizedQuery"`
	MeanMs          float64  `json:"meanMs"`
	TotalMs         float64  `json:"totalMs"`
	Calls           int      `json:"calls"`
	Rows            int      `json:"rows"`
	Endpoints       []string `json:"endpoints"`
	Trend           string   `json:"trend"`
}

func (s *Server) handleAdminSecurityAdvisor(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, securityAdvisorReport{
		EvaluatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Stale:       false,
		Findings:    []securityFinding{},
	})
}

func normalizeAdvisorRange(rawRange string) string {
	switch rawRange {
	case "15m", "1h", "6h", "24h", "7d":
		return rawRange
	default:
		return "1h"
	}
}

func (s *Server) handleAdminPerformanceAdvisor(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, performanceAdvisorReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Stale:       false,
		Range:       normalizeAdvisorRange(r.URL.Query().Get("range")),
		Queries:     []performanceQuery{},
	})
}
