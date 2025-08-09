package user

import (
    "encoding/json"
    "net/http"
    "net/url"
    "strings"

    "auth-service/internal/services"
    "auth-service/pkg/errors"
    "github.com/golang-jwt/jwt/v5"
)

type ManageHandler struct {
    authService *services.AuthService
    userService *services.UserService
}

func NewManageHandler(authService *services.AuthService, userService *services.UserService) *ManageHandler {
    return &ManageHandler{authService: authService, userService: userService}
}

// parseToken returns claims map or writes error.
func (h *ManageHandler) parseToken(w http.ResponseWriter, r *http.Request) jwt.MapClaims {
    authHeader := r.Header.Get("Authorization")
    if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
        errors.WriteError(w, errors.Unauthorized("missing or invalid authorization header"))
        return nil
    }
    tokenStr := authHeader[7:]
    tok, err := h.authService.ValidateToken(tokenStr)
    if err != nil || !tok.Valid {
        errors.WriteError(w, errors.Unauthorized("invalid token"))
        return nil
    }
    claims, ok := tok.Claims.(jwt.MapClaims)
    if !ok {
        errors.WriteError(w, errors.Unauthorized("invalid claims"))
        return nil
    }
    return claims
}

// HandleList lists users with pagination/search (resident+).
func (h *ManageHandler) HandleList(w http.ResponseWriter, r *http.Request) {
    claims := h.parseToken(w, r)
    if claims == nil { return }
    role := claims["role"].(string)
    if role != "resident" && role != "admin" { errors.WriteError(w, errors.Forbidden("forbidden")); return }

    users, meta, err := h.userService.ListUsers(r.URL.Query(), r.Header.Get("Authorization"))
    if err != nil { errors.WriteError(w, errors.InternalServerError("failed to list users", err)); return }
    resp := map[string]interface{}{"users": users}
    for k,v := range meta { resp[k] = v }
    w.Header().Set("Content-Type","application/json")
    json.NewEncoder(w).Encode(resp)
}

// HandleGet fetches a single user by id (any authenticated user can fetch self; resident/admin can fetch others).
func (h *ManageHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
    claims := h.parseToken(w, r); if claims == nil { return }
    requester := claims["sub"].(string)
    role := claims["role"].(string)
    id := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
    if id == "" { errors.WriteError(w, errors.BadRequest("missing id")); return }
    if requester != id && role != "resident" && role != "admin" { errors.WriteError(w, errors.Forbidden("forbidden")); return }
    user, err := h.userService.GetUser(id)
    if err != nil { errors.WriteError(w, errors.NotFound("user not found")); return }
    w.Header().Set("Content-Type","application/json"); json.NewEncoder(w).Encode(user)
}

// HandlePatch updates limited user fields.
func (h *ManageHandler) HandlePatch(w http.ResponseWriter, r *http.Request) {
    claims := h.parseToken(w, r); if claims == nil { return }
    requester := claims["sub"].(string)
    role := claims["role"].(string)
    id := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
    if idx := strings.Index(id, "/"); idx != -1 { id = id[:idx] }
    if id == "" { errors.WriteError(w, errors.BadRequest("missing id")); return }
    var updates map[string]interface{}
    if err := json.NewDecoder(r.Body).Decode(&updates); err != nil { errors.WriteError(w, errors.BadRequest("invalid body")); return }

    if newRoleRaw, ok := updates["role"]; ok {
        newRole, _ := newRoleRaw.(string); newRole = strings.ToLower(newRole)
        if role == "admin" {
            if newRole != "user" && newRole != "resident" && newRole != "admin" { errors.WriteError(w, errors.BadRequest("invalid role")); return }
        } else if role == "resident" {
            if newRole != "resident" { errors.WriteError(w, errors.Forbidden("residents can only grant resident")); return }
        } else { errors.WriteError(w, errors.Forbidden("cannot change role")); return }
    }
    if requester != id && role != "resident" && role != "admin" { errors.WriteError(w, errors.Forbidden("forbidden")); return }

    bearer := r.Header.Get("Authorization")
    if err := h.userService.UpdateUser(id, updates, strings.TrimPrefix(bearer, "Bearer ")); err != nil {
        errors.WriteError(w, errors.InternalServerError("failed to update user", err)); return }
    w.WriteHeader(http.StatusOK); json.NewEncoder(w).Encode(map[string]string{"status":"ok"})
}

// HandleLockout admin only.
func (h *ManageHandler) HandleLockout(w http.ResponseWriter, r *http.Request) {
    claims := h.parseToken(w, r); if claims == nil { return }
    role := claims["role"].(string)
    if role != "admin" { errors.WriteError(w, errors.Forbidden("admin only")); return }
    id := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
    if idx := strings.Index(id, "/"); idx != -1 { id = id[:idx] }
    var req struct { Lock bool `json:"lock"` }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil { errors.WriteError(w, errors.BadRequest("invalid body")); return }
    updates := map[string]interface{}{ "lockout_enabled": req.Lock }
    bearer := r.Header.Get("Authorization")
    if err := h.userService.UpdateUser(id, updates, strings.TrimPrefix(bearer, "Bearer ")); err != nil { errors.WriteError(w, errors.InternalServerError("failed to update lockout", err)); return }
    w.WriteHeader(http.StatusOK); json.NewEncoder(w).Encode(map[string]any{"status":"ok","lock":req.Lock})
}

// Helper to build query params (not used yet)
func buildQuery(q url.Values) string { return q.Encode() }
