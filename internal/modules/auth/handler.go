package auth

import (
	"net/http"

	"github.com/example/goapp/internal/shared/httpx"
	"github.com/example/goapp/internal/shared/validator"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	uc *Usecase
}

func NewHandler(uc *Usecase) *Handler {
	return &Handler{uc: uc}
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a new user account and returns access + refresh tokens.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RegisterRequest        true  "Registration payload"
// @Success      201   {object}  TokenResponse
// @Failure      400   {object}  httpx.ErrorResponse
// @Failure      409   {object}  httpx.ErrorResponse
// @Failure      500   {object}  httpx.ErrorResponse
// @Router       /auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, httpx.BadRequest("request.malformed", "invalid request body"))
		return
	}
	if details, err := validator.Struct(req); err != nil {
		httpx.ErrorWithDetails(c, httpx.BadRequest("request.invalid", "validation failed"), details)
		return
	}
	tokens, err := h.uc.Register(c.Request.Context(), req)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, tokens)
}

// Login godoc
// @Summary      Login
// @Description  Authenticates a user by email + password and returns tokens.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LoginRequest           true  "Login payload"
// @Success      200   {object}  TokenResponse
// @Failure      400   {object}  httpx.ErrorResponse
// @Failure      401   {object}  httpx.ErrorResponse
// @Failure      500   {object}  httpx.ErrorResponse
// @Router       /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, httpx.BadRequest("request.malformed", "invalid request body"))
		return
	}
	if details, err := validator.Struct(req); err != nil {
		httpx.ErrorWithDetails(c, httpx.BadRequest("request.invalid", "validation failed"), details)
		return
	}
	tokens, err := h.uc.Login(c.Request.Context(), req)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, tokens)
}

// Refresh godoc
// @Summary      Refresh access token
// @Description  Exchanges a valid refresh token for a new access + refresh pair.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RefreshRequest         true  "Refresh payload"
// @Success      200   {object}  TokenResponse
// @Failure      400   {object}  httpx.ErrorResponse
// @Failure      401   {object}  httpx.ErrorResponse
// @Failure      500   {object}  httpx.ErrorResponse
// @Router       /auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, httpx.BadRequest("request.malformed", "invalid request body"))
		return
	}
	if details, err := validator.Struct(req); err != nil {
		httpx.ErrorWithDetails(c, httpx.BadRequest("request.invalid", "validation failed"), details)
		return
	}
	tokens, err := h.uc.Refresh(c.Request.Context(), req)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, tokens)
}
