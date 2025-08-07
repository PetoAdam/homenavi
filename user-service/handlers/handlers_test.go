package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"user-service/db"
	"user-service/middleware"
	"user-service/pkg/roles"
	
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	database, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	database.AutoMigrate(&db.User{})
	return database
}

func createTestUser(database *gorm.DB, role string) *db.User {
	user := &db.User{
		ID:                 uuid.New(),
		UserName:           "test" + role,
		NormalizedUserName: strings.ToUpper("test" + role),
		Email:              "test" + role + "@example.com",
		NormalizedEmail:    strings.ToUpper("test" + role + "@example.com"),
		FirstName:          "Test",
		LastName:           role,
		Role:               role,
		EmailConfirmed:     true,
		LockoutEnabled:     false,
		AccessFailedCount:  0,
	}
	database.Create(user)
	return user
}

func createTestJWT(userID, role string) string {
	claims := &middleware.Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestRoleAssignment(t *testing.T) {
	// Set up test database
	originalDB := db.DB
	db.DB = setupTestDB()
	defer func() { db.DB = originalDB }()

	// Create test users
	adminUser := createTestUser(db.DB, roles.Admin)
	residentUser := createTestUser(db.DB, roles.Resident)
	regularUser := createTestUser(db.DB, roles.User)

	tests := []struct {
		name           string
		requestingUser *db.User
		targetUserID   string
		newRole        string
		expectedStatus int
	}{
		{
			name:           "Admin can promote user to resident",
			requestingUser: adminUser,
			targetUserID:   regularUser.ID.String(),
			newRole:        roles.Resident,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Admin can promote user to admin",
			requestingUser: adminUser,
			targetUserID:   regularUser.ID.String(),
			newRole:        roles.Admin,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Resident can promote user to resident",
			requestingUser: residentUser,
			targetUserID:   regularUser.ID.String(),
			newRole:        roles.Resident,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Resident cannot promote user to admin",
			requestingUser: residentUser,
			targetUserID:   regularUser.ID.String(),
			newRole:        roles.Admin,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "User cannot promote anyone",
			requestingUser: regularUser,
			targetUserID:   regularUser.ID.String(),
			newRole:        roles.Resident,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Cannot assign invalid role",
			requestingUser: adminUser,
			targetUserID:   regularUser.ID.String(),
			newRole:        "invalid_role",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request body
			reqBody := map[string]interface{}{
				"role": tt.newRole,
			}
			jsonBody, _ := json.Marshal(reqBody)

			// Create request
			req := httptest.NewRequest("PATCH", "/user/"+tt.targetUserID, bytes.NewReader(jsonBody))
			req.Header.Set("Authorization", "Bearer "+createTestJWT(tt.requestingUser.ID.String(), tt.requestingUser.Role))
			
			// Create response recorder
			w := httptest.NewRecorder()

			// Create router and add route
			r := chi.NewRouter()
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Mock JWT middleware - extract token and set claims
					authHeader := r.Header.Get("Authorization")
					if strings.HasPrefix(authHeader, "Bearer ") {
						tokenString := authHeader[7:]
						token, _ := jwt.ParseWithClaims(tokenString, &middleware.Claims{}, func(token *jwt.Token) (interface{}, error) {
							return []byte("test-secret"), nil
						})
						if claims, ok := token.Claims.(*middleware.Claims); ok {
							ctx := r.Context()
							// Set claims in context using the same key as the actual middleware
							r = r.WithContext(middleware.SetTestClaims(ctx, claims))
						}
					}
					next.ServeHTTP(w, r)
				})
			})
			r.Patch("/user/{id}", HandleUserPatch)

			// Make the request
			r.ServeHTTP(w, req)

			// Check response
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			// If successful, verify the role was actually changed
			if w.Code == http.StatusOK {
				var updatedUser db.User
				db.DB.First(&updatedUser, "id = ?", tt.targetUserID)
				if updatedUser.Role != tt.newRole {
					t.Errorf("Expected role to be updated to %s, but got %s", tt.newRole, updatedUser.Role)
				}
			}
		})
	}
}