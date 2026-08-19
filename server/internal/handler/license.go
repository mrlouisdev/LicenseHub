package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tabloy/keygate/internal/service"
	"github.com/tabloy/keygate/pkg/apperr"
	"github.com/tabloy/keygate/pkg/response"
)

const maxIdentifierLen = 256

type LicenseHandler struct {
	svc *service.LicenseService
}

func NewLicenseHandler(svc *service.LicenseService) *LicenseHandler {
	return &LicenseHandler{svc: svc}
}

func (h *LicenseHandler) Activate(c *gin.Context) {
	var req struct {
		LicenseKey     string `json:"license_key" binding:"required"`
		Identifier     string `json:"identifier" binding:"required"`
		IdentifierType string `json:"identifier_type"`
		Label          string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "license_key and identifier are required")
		return
	}
	req.Identifier = strings.TrimSpace(req.Identifier)
	if len(req.Identifier) > maxIdentifierLen {
		response.BadRequest(c, "identifier too long")
		return
	}

	productID, _ := c.Get("product_id")
	result, err := h.svc.Activate(c.Request.Context(), service.ActivateInput{
		LicenseKey:     req.LicenseKey,
		Identifier:     req.Identifier,
		IdentifierType: req.IdentifierType,
		Label:          req.Label,
		IPAddress:      c.ClientIP(),
		ProductID:      str(productID),
	})
	if err != nil {
		writeAppErr(c, err)
		return
	}
	response.OK(c, result)
}

func (h *LicenseHandler) Verify(c *gin.Context) {
	var req struct {
		LicenseKey string `json:"license_key" binding:"required"`
		Identifier string `json:"identifier" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "license_key and identifier are required")
		return
	}
	req.Identifier = strings.TrimSpace(req.Identifier)
	if len(req.Identifier) > maxIdentifierLen {
		response.BadRequest(c, "identifier too long")
		return
	}

	productID, _ := c.Get("product_id")
	result, err := h.svc.Verify(c.Request.Context(), service.VerifyInput{
		LicenseKey: req.LicenseKey,
		Identifier: req.Identifier,
		ProductID:  str(productID),
		IPAddress:  c.ClientIP(),
	})
	if err != nil {
		writeAppErr(c, err)
		return
	}
	response.OK(c, result)
}

func (h *LicenseHandler) Deactivate(c *gin.Context) {
	var req struct {
		LicenseKey string `json:"license_key" binding:"required"`
		Identifier string `json:"identifier" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "license_key and identifier are required")
		return
	}
	req.Identifier = strings.TrimSpace(req.Identifier)
	if len(req.Identifier) > maxIdentifierLen {
		response.BadRequest(c, "identifier too long")
		return
	}

	productID, _ := c.Get("product_id")
	err := h.svc.Deactivate(c.Request.Context(), service.DeactivateInput{
		LicenseKey: req.LicenseKey,
		Identifier: req.Identifier,
		ProductID:  str(productID),
		IPAddress:  c.ClientIP(),
	})
	if err != nil {
		writeAppErr(c, err)
		return
	}
	response.OK(c, gin.H{"status": "deactivated"})
}

// ClientActivate is the stable LicenseHub SDK adapter. It keeps the upstream
// /api/v1/license endpoints intact while exposing the universal client contract.
func (h *LicenseHandler) ClientActivate(c *gin.Context) {
	req, ok := bindClientLicenseRequest(c)
	if !ok {
		return
	}
	result, err := h.svc.Activate(c.Request.Context(), service.ActivateInput{
		LicenseKey: req.LicenseKey, Identifier: req.DeviceID,
		IdentifierType: "device", Label: req.Label,
		IPAddress: c.ClientIP(), ProductID: req.ProductID,
	})
	if err != nil {
		writeAppErr(c, err)
		return
	}
	c.JSON(200, gin.H{"lease": result.Token, "status": result.Status, "license_id": result.LicenseID})
}

// ClientRefresh verifies the current device activation and mints a fresh lease.
func (h *LicenseHandler) ClientRefresh(c *gin.Context) {
	req, ok := bindClientLeaseRequest(c)
	if !ok {
		return
	}
	lease, err := h.svc.RefreshLease(c.Request.Context(), service.LeaseInput{
		Lease: req.Lease, Identifier: req.DeviceID,
		IPAddress: c.ClientIP(), ProductID: req.ProductID,
	})
	if err != nil {
		writeAppErr(c, err)
		return
	}
	c.JSON(200, gin.H{"lease": lease})
}

func (h *LicenseHandler) ClientDeactivate(c *gin.Context) {
	req, ok := bindClientLeaseRequest(c)
	if !ok {
		return
	}
	if err := h.svc.DeactivateLease(c.Request.Context(), service.LeaseInput{
		Lease: req.Lease, Identifier: req.DeviceID,
		IPAddress: c.ClientIP(), ProductID: req.ProductID,
	}); err != nil {
		writeAppErr(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "deactivated"})
}

type clientLeaseRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	DeviceID  string `json:"device_id" binding:"required"`
	Lease     string `json:"lease" binding:"required"`
}

func bindClientLeaseRequest(c *gin.Context) (clientLeaseRequest, bool) {
	var req clientLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "product_id, device_id and lease are required")
		return req, false
	}
	req.ProductID = strings.TrimSpace(req.ProductID)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.ProductID == "" || req.DeviceID == "" || len(req.DeviceID) > maxIdentifierLen {
		response.BadRequest(c, "invalid product_id or device_id")
		return req, false
	}
	return req, true
}

type clientLicenseRequest struct {
	LicenseKey string `json:"license_key" binding:"required"`
	ProductID  string `json:"product_id" binding:"required"`
	DeviceID   string `json:"device_id" binding:"required"`
	Label      string `json:"label"`
}

func bindClientLicenseRequest(c *gin.Context) (clientLicenseRequest, bool) {
	var req clientLicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "license_key, product_id and device_id are required")
		return req, false
	}
	req.ProductID = strings.TrimSpace(req.ProductID)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.ProductID == "" || req.DeviceID == "" || len(req.DeviceID) > maxIdentifierLen {
		response.BadRequest(c, "invalid product_id or device_id")
		return req, false
	}
	return req, true
}

// writeAppErr translates an AppError into a consistent API response.
func writeAppErr(c *gin.Context, err error) {
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		if ae.Details != nil {
			response.ErrWithDetails(c, ae.Status, ae.Code, ae.Message, ae.Details)
		} else {
			response.Err(c, ae.Status, ae.Code, ae.Message)
		}
		return
	}
	response.Internal(c)
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
