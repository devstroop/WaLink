package handler

import (
	"net/http"
	"time"

	"github.com/devstroop/walink/internal/database"
	"github.com/devstroop/walink/internal/middleware"
	"github.com/devstroop/walink/internal/model"
)

// BillingHandler handles plan, subscription, and usage endpoints.
type BillingHandler struct {
	db *database.DB
}

// NewBillingHandler creates a new billing handler.
func NewBillingHandler(db *database.DB) *BillingHandler {
	return &BillingHandler{db: db}
}

// ListPlans — GET /api/v1/billing/plans (public, no auth required)
func (h *BillingHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	recs, err := h.db.ListPlans()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}

	plans := make([]model.PlanInfo, 0, len(recs))
	for _, rec := range recs {
		plans = append(plans, model.PlanInfo{
			ID:          rec.ID,
			Name:        rec.Name,
			Description: rec.Description,
			PriceCents:  rec.PriceCents,
			Interval:    rec.Interval,
			Limits:      rec.PlanLimits(),
			IsDefault:   rec.IsDefault,
		})
	}

	writeJSON(w, http.StatusOK, model.PlanListResponse{Plans: plans, Total: len(plans)})
}

// GetBilling — GET /api/v1/billing
func (h *BillingHandler) GetBilling(w http.ResponseWriter, r *http.Request) {
	id := middleware.GetIdentity(r)
	if id == nil {
		writeError(w, http.StatusUnauthorized, "missing authorization")
		return
	}

	limits, planID, _ := h.db.GetUserPlanLimits(id.UserID)

	plan, err := h.db.GetPlan(planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get plan")
		return
	}

	sub, _ := h.db.GetSubscription(id.UserID)
	subInfo := model.SubscriptionInfo{PlanID: planID, PlanName: plan.Name, Status: "active"}
	if sub != nil {
		subInfo = model.SubscriptionInfo{
			ID:                 sub.ID,
			PlanID:             sub.PlanID,
			PlanName:           plan.Name,
			Status:             sub.Status,
			CurrentPeriodStart: sub.CurrentPeriodStart,
			CurrentPeriodEnd:   sub.CurrentPeriodEnd,
			CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
		}
	}

	dailyUsage, _ := h.db.GetDailyUsage(id.UserID)
	acctCount, _ := h.db.CountUserAccounts(id.UserID)

	usage := model.UsageInfo{
		Date:         time.Now().UTC().Format("2006-01-02"),
		MessagesSent: dailyUsage,
		DailyLimit:   limits.DailyMessages,
		AccountsUsed: acctCount,
		AccountLimit: limits.MaxAccounts,
	}

	writeJSON(w, http.StatusOK, model.BillingResponse{
		Subscription: subInfo,
		Usage:        usage,
		Plan: model.PlanInfo{
			ID:          plan.ID,
			Name:        plan.Name,
			Description: plan.Description,
			PriceCents:  plan.PriceCents,
			Interval:    plan.Interval,
			Limits:      plan.PlanLimits(),
			IsDefault:   plan.IsDefault,
		},
	})
}

// GetUsage — GET /api/v1/billing/usage
func (h *BillingHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	id := middleware.GetIdentity(r)
	if id == nil {
		writeError(w, http.StatusUnauthorized, "missing authorization")
		return
	}

	limits, _, _ := h.db.GetUserPlanLimits(id.UserID)
	dailyUsage, _ := h.db.GetDailyUsage(id.UserID)
	acctCount, _ := h.db.CountUserAccounts(id.UserID)

	writeJSON(w, http.StatusOK, model.UsageInfo{
		Date:         time.Now().UTC().Format("2006-01-02"),
		MessagesSent: dailyUsage,
		DailyLimit:   limits.DailyMessages,
		AccountsUsed: acctCount,
		AccountLimit: limits.MaxAccounts,
	})
}

// RegisterBillingRoutes wires billing endpoints into the mux.
// Plans listing is public (on the main mux); the rest require auth.
func RegisterBillingRoutes(publicMux *http.ServeMux, authedMux *http.ServeMux, db *database.DB) {
	billing := NewBillingHandler(db)

	// Public (no auth) — browsing available plans
	publicMux.HandleFunc("GET /api/v1/billing/plans", billing.ListPlans)

	// Authenticated
	authedMux.HandleFunc("GET /api/v1/billing", billing.GetBilling)
	authedMux.HandleFunc("GET /api/v1/billing/usage", billing.GetUsage)
}
