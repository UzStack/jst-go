package server

import (
	"net/http"

	"github.com/UzStack/jst-go/internal/modules/auth"
	"github.com/UzStack/jst-go/internal/modules/user"
	"github.com/UzStack/jst-go/internal/shared/buildinfo"
	"github.com/UzStack/jst-go/internal/shared/config"
	"github.com/UzStack/jst-go/internal/shared/logger"
	"github.com/UzStack/jst-go/internal/shared/middleware"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	r.Use(
		middleware.RequestID(),
		middleware.Recovery(log),
		middleware.Logger(log),
		middleware.CORS(cfg.HTTP.CORSOrigins),
		middleware.RateLimit(cfg.HTTP.RateLimitRPS, cfg.HTTP.RateLimitBurst),
		middleware.BodyLimit(cfg.HTTP.MaxBodyBytes),
		middleware.Timeout(cfg.HTTP.RequestTimeout),
	)

	s := &Server{cfg: cfg, log: log, pool: pool, router: r}
	s.registerRoutes()
	return s
}

func (s *Server) Router() http.Handler { return s.router }

func (s *Server) registerRoutes() {
	// Liveness: process is up. Cheap, never touches the DB.
	s.router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": buildinfo.Version})
	})

	// Readiness: dependencies are reachable. Pings the DB so an orchestrator
	// stops routing traffic when Postgres is down.
	s.router.GET("/readyz", func(c *gin.Context) {
		if err := s.pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "db": "down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Swagger UI: /swagger/index.html — disabled in production by default.
	if s.cfg.Env != "production" {
		s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	v1 := s.router.Group("/api/v1")

	// user module
	userRepo := user.NewPostgresRepository(s.pool)
	userUC := user.NewUsecase(userRepo)

	// auth module — depends on user usecase + jwt issuer + refresh store
	tokens := auth.NewTokenIssuer(s.cfg.JWT)
	refreshStore := auth.NewRefreshStore(s.pool)
	authUC := auth.NewUsecase(userUC, tokens, refreshStore)
	auth.RegisterRoutes(v1, auth.NewHandler(authUC))

	// user routes need the verifier from auth
	user.RegisterRoutes(v1, user.NewHandler(userUC), tokens)
}
