package roles

import "testing"

func TestRoleHierarchy(t *testing.T) {
	// Test role levels
	tests := []struct {
		role     string
		expected int
	}{
		{Public, 0},
		{User, 1},
		{Resident, 2},
		{Admin, 3},
		{"invalid", -1},
	}

	for _, test := range tests {
		if level := GetRoleLevel(test.role); level != test.expected {
			t.Errorf("GetRoleLevel(%s) = %d, want %d", test.role, level, test.expected)
		}
	}
}

func TestCanAssignRole(t *testing.T) {
	tests := []struct {
		fromRole string
		toRole   string
		expected bool
	}{
		// Admin can assign any role
		{Admin, Admin, true},
		{Admin, Resident, true},
		{Admin, User, true},
		{Admin, Public, true},
		
		// Resident can assign resident and user roles
		{Resident, Resident, true},
		{Resident, User, true},
		{Resident, Public, true},
		{Resident, Admin, false},
		
		// User can only assign user role
		{User, User, true},
		{User, Public, true},
		{User, Resident, false},
		{User, Admin, false},
		
		// Public cannot assign any role
		{Public, Public, true},
		{Public, User, false},
		{Public, Resident, false},
		{Public, Admin, false},
		
		// Invalid roles
		{"invalid", User, false},
		{User, "invalid", false},
	}

	for _, test := range tests {
		result := CanAssignRole(test.fromRole, test.toRole)
		if result != test.expected {
			t.Errorf("CanAssignRole(%s, %s) = %t, want %t", 
				test.fromRole, test.toRole, result, test.expected)
		}
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		userRole     string
		requiredRole string
		expected     bool
	}{
		// Admin has all permissions
		{Admin, Admin, true},
		{Admin, Resident, true},
		{Admin, User, true},
		{Admin, Public, true},
		
		// Resident has resident and below permissions
		{Resident, Admin, false},
		{Resident, Resident, true},
		{Resident, User, true},
		{Resident, Public, true},
		
		// User has user and below permissions
		{User, Admin, false},
		{User, Resident, false},
		{User, User, true},
		{User, Public, true},
		
		// Public has only public permissions
		{Public, Admin, false},
		{Public, Resident, false},
		{Public, User, false},
		{Public, Public, true},
	}

	for _, test := range tests {
		result := HasPermission(test.userRole, test.requiredRole)
		if result != test.expected {
			t.Errorf("HasPermission(%s, %s) = %t, want %t", 
				test.userRole, test.requiredRole, result, test.expected)
		}
	}
}

func TestIsValidRole(t *testing.T) {
	validRoles := []string{Public, User, Resident, Admin}
	for _, role := range validRoles {
		if !IsValidRole(role) {
			t.Errorf("IsValidRole(%s) should be true", role)
		}
	}
	
	invalidRoles := []string{"invalid", "", "ADMIN", "admin "}
	for _, role := range invalidRoles {
		if IsValidRole(role) {
			t.Errorf("IsValidRole(%s) should be false", role)
		}
	}
}