// Package server wires the Gin router. Same code path is used by main.go
// and by the integration test suite.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/auth"
	"github.com/harshit/food-ordering-app/internal/config"
	"github.com/harshit/food-ordering-app/internal/handlers"
	"github.com/harshit/food-ordering-app/internal/mailer"
	"github.com/harshit/food-ordering-app/internal/middleware"
)

// New builds the API router with all routes wired. CORS allows the Vite dev
// origin by default; pass a custom allowedOrigin if needed.
func New(cfg *config.Config, db *gorm.DB, mail *mailer.Mailer) (*gin.Engine, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	jwtCfg := auth.JWTConfig{
		Secret: []byte(cfg.JWTSecret),
		TTL:    time.Duration(cfg.JWTTTLDays) * 24 * time.Hour,
	}

	isProd := cfg.IsProduction()

	publicH := handlers.NewPublicHandler(db)
	authH := handlers.NewAuthHandler(db, jwtCfg, cfg.OTPTTLSeconds, mail, isProd)
	orderH := handlers.NewOrderHandler(db)
	adminAuthH := handlers.NewAdminAuthHandler(db, jwtCfg, cfg.OTPTTLSeconds, mail, isProd)
	adminOrderH := handlers.NewAdminOrderHandler(db)
	adminStaffH := handlers.NewAdminStaffHandler(db)
	adminMenuH := handlers.NewAdminMenuHandler(db)
	adminTableH := handlers.NewAdminTableHandler(db)
	superOrgsH := handlers.NewSuperOrgsHandler(db)
	superStaffH := handlers.NewSuperStaffHandler(db)

	customerAuth := middleware.CustomerAuth(db, jwtCfg)
	staffAuth := middleware.StaffAuth(db, jwtCfg)
	requireAdmin := middleware.RequireAdmin()
	requireSuperAdmin := middleware.RequireSuperAdmin()

	r := gin.New()
	r.Use(gin.Recovery())
	if gin.Mode() != gin.TestMode {
		r.Use(gin.Logger())
	}
	corsOrigins := cfg.CORSOrigins
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:5173"}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowMethods:     []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	api := r.Group("/api/v1")

	api.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "db ping failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Public — no auth
	api.GET("/public/context", publicH.GetContext)
	api.POST("/auth/otp/request", authH.RequestOTP)
	api.POST("/auth/otp/verify", authH.VerifyOTP)

	// Customer-authenticated
	authed := api.Group("")
	authed.Use(customerAuth)
	{
		authed.GET("/me", authH.Me)
		authed.POST("/auth/logout", authH.Logout)
		authed.GET("/orgs/:org_id/menu", publicH.GetMenu)
		authed.GET("/orgs/:org_id/tables/:table_code/active-order", orderH.GetActiveOrder)
		authed.POST("/orders", orderH.PlaceOrAppend)
		authed.GET("/orders/:public_code", orderH.GetByPublicCode)
	}

	// Admin/staff
	api.POST("/admin/auth/otp/request", adminAuthH.RequestOTP)
	api.POST("/admin/auth/otp/verify", adminAuthH.VerifyOTP)

	admin := api.Group("/admin")
	admin.Use(staffAuth)
	{
		admin.GET("/me", adminAuthH.Me)
		admin.POST("/auth/logout", adminAuthH.Logout)

		admin.GET("/orders/active", adminOrderH.ListActive)
		admin.GET("/orders/history", adminOrderH.ListHistory)
		admin.GET("/orders/:public_code", adminOrderH.GetByCode)
		admin.PATCH("/orders/:public_code/status", adminOrderH.UpdateStatus)
		admin.POST("/orders/:public_code/complete", adminOrderH.Complete)

		admin.GET("/staff", adminStaffH.List)
		admin.POST("/staff", requireAdmin, adminStaffH.Create)
		admin.PATCH("/staff/:id", requireAdmin, adminStaffH.Update)

		admin.GET("/menu", requireAdmin, adminMenuH.List)
		admin.POST("/menu", requireAdmin, adminMenuH.Create)
		admin.PATCH("/menu/:id", requireAdmin, adminMenuH.Update)
		admin.POST("/menu/:id/image", requireAdmin, adminMenuH.UploadImage)

		admin.GET("/tables", requireAdmin, adminTableH.List)
		admin.POST("/tables", requireAdmin, adminTableH.Create)
		admin.PATCH("/tables/:id", requireAdmin, adminTableH.Update)
	}

	// Platform-level (super_admin only)
	super := api.Group("/super")
	super.Use(staffAuth, requireSuperAdmin)
	{
		super.GET("/orgs", superOrgsH.ListOrgs)
		super.POST("/orgs", superOrgsH.CreateOrgWithManager)
		super.PATCH("/orgs/:id", superOrgsH.UpdateOrg)

		super.GET("/staff", superStaffH.ListAll)
		super.POST("/staff", superStaffH.Create)
		super.PATCH("/staff/:id", superStaffH.Update)
	}

	r.Static("/uploads", "./uploads")
	return r, nil
}
