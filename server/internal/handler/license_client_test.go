package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func clientContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/client/activate", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestBindClientLicenseRequestStrict(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		c, w := clientContext(`{"license_key":" key ","product_id":" prod ","device_id":" dev ","label":" laptop "}`)
		req, ok := bindClientLicenseRequest(c)
		if !ok || w.Code != http.StatusOK {
			t.Fatalf("ok/status = %v/%d", ok, w.Code)
		}
		if req.LicenseKey != "key" || req.ProductID != "prod" || req.DeviceID != "dev" || req.Label != "laptop" {
			t.Fatalf("request was not normalized: %#v", req)
		}
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		c, w := clientContext(`{"license_key":"key","product_id":"prod","device_id":"dev","admin":true}`)
		if _, ok := bindClientLicenseRequest(c); ok || w.Code != http.StatusBadRequest {
			t.Fatalf("ok/status = %v/%d", ok, w.Code)
		}
	})

	t.Run("oversized body rejected", func(t *testing.T) {
		c, w := clientContext(`{"license_key":"` + strings.Repeat("x", maxClientRequestBody) + `","product_id":"prod","device_id":"dev"}`)
		if _, ok := bindClientLicenseRequest(c); ok || w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("ok/status = %v/%d", ok, w.Code)
		}
	})
}

func TestBindClientLeaseRequestBounds(t *testing.T) {
	c, w := clientContext(`{"product_id":"prod","device_id":"dev","lease":"` + strings.Repeat("x", maxClientLease+1) + `"}`)
	if _, ok := bindClientLeaseRequest(c); ok || w.Code != http.StatusBadRequest {
		t.Fatalf("ok/status = %v/%d", ok, w.Code)
	}
}
