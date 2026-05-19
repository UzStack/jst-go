package user

import (
	"github.com/example/goapp/internal/shared/httpx"
	"github.com/example/goapp/internal/shared/middleware"
	"github.com/example/goapp/internal/shared/validator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler {
	return &Handler{uc: uc}
}

// Me godoc
// @Summary      Get current user
// @Description  Returns the authenticated user's profile.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  UserResponse
// @Failure      401  {object}  httpx.ErrorResponse
// @Failure      404  {object}  httpx.ErrorResponse
// @Router       /users/me [get]
func (h *Handler) Me(c *gin.Context) {
	uid, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		httpx.Error(c, httpx.Unauthorized("auth.invalid_token", "invalid token"))
		return
	}
	u, err := h.uc.GetByID(c.Request.Context(), uid)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, 200, toResponse(u))
}

// UpdateMe godoc
// @Summary      Update current user
// @Description  Updates the authenticated user's name.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      UpdateMeRequest        true  "Update payload"
// @Success      200   {object}  UserResponse
// @Failure      400   {object}  httpx.ErrorResponse
// @Failure      401   {object}  httpx.ErrorResponse
// @Failure      404   {object}  httpx.ErrorResponse
// @Router       /users/me [patch]
func (h *Handler) UpdateMe(c *gin.Context) {
	uid, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		httpx.Error(c, httpx.Unauthorized("auth.invalid_token", "invalid token"))
		return
	}

	var req UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, httpx.BadRequest("request.malformed", "invalid request body"))
		return
	}
	if details, err := validator.Struct(req); err != nil {
		httpx.ErrorWithDetails(c, httpx.BadRequest("request.invalid", "validation failed"), details)
		return
	}

	u, err := h.uc.UpdateName(c.Request.Context(), uid, req.Name)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, 200, toResponse(u))
}

// DeleteMe godoc
// @Summary      Delete current user
// @Description  Permanently deletes the authenticated user.
// @Tags         users
// @Security     BearerAuth
// @Success      204
// @Failure      401  {object}  httpx.ErrorResponse
// @Failure      404  {object}  httpx.ErrorResponse
// @Router       /users/me [delete]
func (h *Handler) DeleteMe(c *gin.Context) {
	uid, err := uuid.Parse(middleware.UserID(c))
	if err != nil {
		httpx.Error(c, httpx.Unauthorized("auth.invalid_token", "invalid token"))
		return
	}
	if err := h.uc.Delete(c.Request.Context(), uid); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.NoContent(c)
}
