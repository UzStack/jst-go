package user

import (
	"strconv"

	"github.com/UzStack/jst-go/internal/shared/httpx"
	"github.com/UzStack/jst-go/internal/shared/middleware"
	"github.com/UzStack/jst-go/internal/shared/validator"
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

// List godoc
// @Summary      List users (admin)
// @Description  Returns a paginated, filterable list of users. Admin only.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        search  query     string  false  "Filter by email or name"
// @Param        limit   query     int     false  "Page size (default 20, max 100)"
// @Param        offset  query     int     false  "Rows to skip (default 0)"
// @Success      200  {object}  ListResponse
// @Failure      401  {object}  httpx.ErrorResponse
// @Failure      403  {object}  httpx.ErrorResponse
// @Router       /users [get]
func (h *Handler) List(c *gin.Context) {
	// ParseInt with bitSize 32 keeps values in int32 range (usecase clamps
	// limit/offset further).
	limit, _ := strconv.ParseInt(c.Query("limit"), 10, 32)
	offset, _ := strconv.ParseInt(c.Query("offset"), 10, 32)
	f := ListFilter{
		Search: c.Query("search"),
		Limit:  int32(limit),
		Offset: int32(offset),
	}
	users, total, err := h.uc.List(c.Request.Context(), f)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, 200, toListResponse(users, total, f.Limit, f.Offset))
}

// GetByID godoc
// @Summary      Get user by id (admin)
// @Description  Returns any user's profile by id. Admin only.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "User id"
// @Success      200  {object}  UserResponse
// @Failure      401  {object}  httpx.ErrorResponse
// @Failure      403  {object}  httpx.ErrorResponse
// @Failure      404  {object}  httpx.ErrorResponse
// @Router       /users/{id} [get]
func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, httpx.BadRequest("user.invalid_id", "invalid user id"))
		return
	}
	u, err := h.uc.GetByID(c.Request.Context(), id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, 200, toResponse(u))
}

// SetRole godoc
// @Summary      Set user role (admin)
// @Description  Changes a user's role. Admin only.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string          true  "User id"
// @Param        body  body      SetRoleRequest  true  "New role"
// @Success      200  {object}  UserResponse
// @Failure      400  {object}  httpx.ErrorResponse
// @Failure      401  {object}  httpx.ErrorResponse
// @Failure      403  {object}  httpx.ErrorResponse
// @Failure      404  {object}  httpx.ErrorResponse
// @Router       /users/{id}/role [patch]
func (h *Handler) SetRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, httpx.BadRequest("user.invalid_id", "invalid user id"))
		return
	}
	var req SetRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, httpx.BadRequest("request.malformed", "invalid request body"))
		return
	}
	if details, err := validator.Struct(req); err != nil {
		httpx.ErrorWithDetails(c, httpx.BadRequest("request.invalid", "validation failed"), details)
		return
	}
	u, err := h.uc.SetRole(c.Request.Context(), id, req.Role)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, 200, toResponse(u))
}
