package server

import (
	"net/http"

	"github.com/example/goapp/internal/modules/auth"
	"github.com/example/goapp/internal/modules/user"
	"github.com/example/goapp/internal/shared/config"
	"github.com/example/goapp/internal/shared/logger"
	"github.com/example/goapp/internal/shared/middleware"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server wires the dependency graph. Adding a new module is a single block
// in registerRoutes — repository, usecase, handler, routes.
type Server struct {
	cfg    *config.Config
	log    *logger.Logger
	pool   *pgxpool.Pool
	router *gin.Engine
}

func New(cfg *config.Config, log *logger.Logger, pool *pgxpool.Pool) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.Recovery(log), middleware.Logger(log))

	s := &Server{cfg: cfg, log: log, pool: pool, router: r}
	s.registerRoutes()
	return s
}

func (s *Server) Router() http.Handler { return s.router }

func (s *Server) registerRoutes() {
	s.router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := s.router.Group("/api/v1")

	// user module
	userRepo := user.NewPostgresRepository(s.pool)
	userUC := user.NewUsecase(userRepo)

	// auth module — depends on user usecase + jwt issuer
	tokens := auth.NewTokenIssuer(s.cfg.JWT)
	authUC := auth.NewUsecase(userUC, tokens)
	auth.RegisterRoutes(v1, auth.NewHandler(authUC))

	// user routes need the verifier from auth
	user.RegisterRoutes(v1, user.NewHandler(userUC), tokens)
}
