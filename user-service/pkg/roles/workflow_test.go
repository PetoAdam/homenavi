package roles

import "testing"

func TestCompleteWorkflow(t *testing.T) {
	// Test the complete workflow of role management
	t.Run("Role hierarchy validation", func(t *testing.T) {
		// Test the hierarchy levels
		if GetRoleLevel(Admin) <= GetRoleLevel(Resident) {
			t.Error("Admin should have higher level than Resident")
		}
		if GetRoleLevel(Resident) <= GetRoleLevel(User) {
			t.Error("Resident should have higher level than User")
		}
		if GetRoleLevel(User) <= GetRoleLevel(Public) {
			t.Error("User should have higher level than Public")
		}
	})

	t.Run("Smart home role scenarios", func(t *testing.T) {
		// Scenario 1: Admin setting up household
		if !CanAssignRole(Admin, Resident) {
			t.Error("Admin should be able to create resident accounts")
		}

		// Scenario 2: Resident inviting family member
		if !CanAssignRole(Resident, Resident) {
			t.Error("Resident should be able to invite other residents")
		}

		// Scenario 3: Resident cannot escalate to admin
		if CanAssignRole(Resident, Admin) {
			t.Error("Resident should not be able to create admin accounts")
		}

		// Scenario 4: Guest/user limitations
		if CanAssignRole(User, Resident) {
			t.Error("Regular user should not be able to promote to resident")
		}
	})

	t.Run("Permission inheritance", func(t *testing.T) {
		// Admin has all permissions
		if !HasPermission(Admin, Public) || !HasPermission(Admin, User) || 
		   !HasPermission(Admin, Resident) || !HasPermission(Admin, Admin) {
			t.Error("Admin should have all permissions")
		}

		// Resident has resident and below permissions
		if !HasPermission(Resident, Public) || !HasPermission(Resident, User) ||
		   !HasPermission(Resident, Resident) {
			t.Error("Resident should have resident and below permissions")
		}
		if HasPermission(Resident, Admin) {
			t.Error("Resident should not have admin permissions")
		}

		// User has user and below permissions
		if !HasPermission(User, Public) || !HasPermission(User, User) {
			t.Error("User should have user and below permissions")
		}
		if HasPermission(User, Resident) || HasPermission(User, Admin) {
			t.Error("User should not have resident or admin permissions")
		}
	})
}

func TestSmartHomeUseCases(t *testing.T) {
	t.Run("Device control permissions", func(t *testing.T) {
		// In a smart home context:
		// - Public users should not control devices
		// - Users (guests) have basic access
		// - Residents control household devices
		// - Admins manage everything

		testCases := []struct {
			userRole     string
			requiredRole string
			description  string
			shouldAllow  bool
		}{
			{Admin, Public, "Admin accessing public info", true},
			{Resident, User, "Resident controlling basic devices", true},
			{User, Resident, "Guest trying to control private devices", false},
			{Public, User, "Unauthenticated access to user features", false},
		}

		for _, tc := range testCases {
			result := HasPermission(tc.userRole, tc.requiredRole)
			if result != tc.shouldAllow {
				t.Errorf("%s: expected %t, got %t", tc.description, tc.shouldAllow, result)
			}
		}
	})
}