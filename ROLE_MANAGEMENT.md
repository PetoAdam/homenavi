# Role Management System Documentation

## Overview

This document describes the enhanced hierarchical role management system implemented for the HomeNavi smart home application.

## Role Hierarchy

The system implements a four-tier role hierarchy:

```
public < user < resident < admin
   0       1       2        3
```

### Role Definitions

- **public**: No authentication required (level 0)
- **user**: Basic authenticated user/guest (level 1)  
- **resident**: Household member with elevated permissions (level 2)
- **admin**: Full system administrator (level 3)

## Role Assignment Rules

The system follows a "can only assign up to your own level" principle:

- **Admin users** can assign: admin, resident, user roles
- **Resident users** can assign: resident, user roles
- **User users** can assign: user roles only
- **Public** cannot assign any roles

## API Changes

### User Service

#### PATCH /api/users/{id}
- Enhanced with role hierarchy validation
- Different authorization logic for role changes vs profile updates
- Role changes require resident+ permissions on the target user
- Role assignment validates the requesting user can assign the target role

### Middleware Enhancements

#### API Gateway
- `AdminOnlyMiddleware`: Updated to use role hierarchy
- `ResidentOrAboveMiddleware`: New middleware for resident+ access
- `RoleMiddleware(requiredRole)`: Flexible role-based middleware

## Database Schema

### User Table
- `role` field: varchar(16) with default 'user'
- Default users created:
  - admin@example.com / admin (role: admin)
  - resident@example.com / resident (role: resident)

## Implementation Details

### Core Components

1. **Role Package** (`pkg/roles/`)
   - Constants for all role types
   - Hierarchy validation functions
   - Permission checking utilities

2. **Authorization Logic**
   - `CanAssignRole(fromRole, toRole)`: Validates role assignment permissions
   - `HasPermission(userRole, requiredRole)`: Checks permission levels
   - `IsValidRole(role)`: Validates role existence

3. **Enhanced Handlers**
   - Separate authorization for role changes vs profile updates
   - Comprehensive role validation before assignment
   - Proper error responses for permission violations

## Testing

Comprehensive test suite includes:
- Unit tests for role hierarchy logic
- Integration tests for role assignment workflows
- Smart home use case validation
- Permission inheritance verification

## Smart Home Context

The `resident` role is specifically designed for smart home applications:

- **Household Members**: People who live in the house get resident role
- **Device Control**: Residents can control household devices and systems
- **User Management**: Residents can invite other residents (family/roommates)
- **Guest Management**: Residents can manage temporary user accounts for guests
- **Admin Separation**: Residents have household control without system admin rights

## Backward Compatibility

- All existing admin functionality preserved
- Existing user role behavior unchanged
- API endpoints maintain same signatures
- Database migration not required (uses existing role field)

## Usage Examples

### Role Assignment

```bash
# Admin promoting user to resident (✅ Allowed)
curl -X PATCH /api/users/123 \
  -H "Authorization: Bearer <admin_token>" \
  -d '{"role": "resident"}'

# Resident promoting user to admin (❌ Forbidden)  
curl -X PATCH /api/users/123 \
  -H "Authorization: Bearer <resident_token>" \
  -d '{"role": "admin"}'

# Resident promoting user to resident (✅ Allowed)
curl -X PATCH /api/users/123 \
  -H "Authorization: Bearer <resident_token>" \
  -d '{"role": "resident"}'
```

### Middleware Usage

```go
// Require resident or higher permissions
r.Route("/household", func(r chi.Router) {
    r.Use(middleware.ResidentOrAboveMiddleware)
    r.Get("/devices", handleDevices)
})

// Flexible role requirement
r.Route("/admin", func(r chi.Router) {
    r.Use(middleware.RoleMiddleware(roles.Admin))
    r.Get("/system", handleSystem)
})
```

## Benefits

1. **Fine-grained Access Control**: Appropriate permissions for smart home context
2. **Household Management**: Residents can manage their household users
3. **Security**: Prevents privilege escalation beyond user's own level
4. **Simplicity**: Uses existing string-based role system without complexity
5. **Extensibility**: Easy to add new roles or modify hierarchy as needed

## Future Enhancements

Potential future improvements:
- Room-based permissions within household
- Temporary role assignments with expiration
- Device-specific role permissions
- Multi-household support for users