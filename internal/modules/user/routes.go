package user

import (
	"github.com/example/goapp/internal/shared/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the user module on the given router group. The auth
// verifier is injected so the module stays decoupled from the auth package.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, verifier middleware.TokenVerifier) {
	g := rg.Group("/users", middleware.Auth(verifier))
	{
		g.GET("/me", h.Me)
		g.PATCH("/me", h.UpdateMe)
		g.DELETE("/me", h.DeleteMe)
	}
}
