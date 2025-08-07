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

// ValidRoles returns a slice of all valid roles
func ValidRoles() []string {
	return []string{User, Resident, Admin}
}

// IsValidRole checks if a role is valid
func IsValidRole(role string) bool {
	_, exists := roleHierarchy[role]
	return exists
}

// GetRoleLevel returns the hierarchy level for a role
func GetRoleLevel(role string) int {
	if level, exists := roleHierarchy[role]; exists {
		return level
	}
	return -1 // Invalid role
}

// CanAssignRole checks if a user with fromRole can assign toRole to another user
func CanAssignRole(fromRole, toRole string) bool {
	fromLevel := GetRoleLevel(fromRole)
	toLevel := GetRoleLevel(toRole)
	
	// Invalid roles cannot assign anything
	if fromLevel == -1 || toLevel == -1 {
		return false
	}
	
	// Can only assign roles up to your own level
	return fromLevel >= toLevel
}

// HasPermission checks if a role has at least the required permission level
func HasPermission(userRole, requiredRole string) bool {
	userLevel := GetRoleLevel(userRole)
	requiredLevel := GetRoleLevel(requiredRole)
	
	if userLevel == -1 || requiredLevel == -1 {
		return false
	}
	
	return userLevel >= requiredLevel
}

// IsEqualOrHigher checks if userRole is equal or higher than compareRole
func IsEqualOrHigher(userRole, compareRole string) bool {
	return GetRoleLevel(userRole) >= GetRoleLevel(compareRole)
}