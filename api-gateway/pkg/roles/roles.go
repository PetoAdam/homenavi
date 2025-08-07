package roles

// Role constants defining the hierarchy
const (
	Public   = "public"   // No authentication required
	User     = "user"     // Basic authenticated user
	Resident = "resident" // Household member/owner
	Admin    = "admin"    // Full system administrator
)

// roleHierarchy defines the role hierarchy levels (higher number = more privileges)
var roleHierarchy = map[string]int{
	Public:   0,
	User:     1,
	Resident: 2,
	Admin:    3,
}

// HasPermission checks if a role has at least the required permission level
func HasPermission(userRole, requiredRole string) bool {
	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]
	
	if !userExists || !requiredExists {
		return false
	}
	
	return userLevel >= requiredLevel
}