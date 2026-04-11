package dashboard

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Repository provides persistence operations for dashboards.
type Repository interface {
	GetDefaultDashboard(context.Context) (*Dashboard, error)
	GetUserDashboard(context.Context, uuid.UUID) (*Dashboard, error)
	CreateDashboard(context.Context, *Dashboard) error
	UpdateUserDashboardDoc(context.Context, uuid.UUID, int, datatypes.JSON) (*Dashboard, error)
	UpsertDefaultDashboard(context.Context, string, any) (*Dashboard, error)
}

type WidgetCatalogSource interface {
	Widgets(context.Context, AuthContext) ([]WidgetType, error)
}
