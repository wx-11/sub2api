package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAffiliateLookupRouter(adminSvc *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	affiliateHandler := NewAffiliateHandler(nil, adminSvc)
	router.GET("/api/v1/admin/affiliates/users/lookup", affiliateHandler.LookupUsers)
	return router
}

func TestAffiliateHandlerLookupUsers_PrioritizesExactEmailMatch(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{
		{ID: 1, Email: "1@qq.com", Role: service.RoleAdmin, Status: service.StatusActive},
		{ID: 2, Email: "other1@qq.com", Role: service.RoleUser, Status: service.StatusActive},
	}
	router := setupAffiliateLookupRouter(adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/affiliates/users/lookup?q=1@qq.com", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []AffiliateUserSummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)
	require.Equal(t, int64(1), resp.Data[0].ID)
	require.Equal(t, "1@qq.com", resp.Data[0].Email)
}

func TestAffiliateHandlerLookupUsers_IncludesDirectIDLookup(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.users = nil
	router := setupAffiliateLookupRouter(adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/affiliates/users/lookup?q=9", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []AffiliateUserSummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, int64(9), resp.Data[0].ID)
}
