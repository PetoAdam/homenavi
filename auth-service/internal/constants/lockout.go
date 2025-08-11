package constants

// Lockout reason constants used across responses so frontend can rely on stable values.
const (
    ReasonLoginLockout = "login_lockout"
    ReasonTwoFALockout = "2fa_lockout"
    ReasonAdminLock    = "admin_lock"
)
