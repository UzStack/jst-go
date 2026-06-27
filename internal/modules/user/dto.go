package user

import "time"

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toResponse(u *User) UserResponse {
	return UserResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Name:      u.Name,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

type UpdateMeRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

type SetRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

// ListResponse is a paginated envelope for a user listing.
type ListResponse struct {
	Items  []UserResponse `json:"items"`
	Total  int64          `json:"total"`
	Limit  int32          `json:"limit"`
	Offset int32          `json:"offset"`
}

func toListResponse(users []User, total int64, limit, offset int32) ListResponse {
	items := make([]UserResponse, len(users))
	for i := range users {
		items[i] = toResponse(&users[i])
	}
	return ListResponse{Items: items, Total: total, Limit: limit, Offset: offset}
}
