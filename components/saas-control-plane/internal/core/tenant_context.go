package core

import (
	"context"

	"github.com/google/uuid"
)

type tenantContextKey struct{}
type platformContextKey struct{}
type tenantProvisioningContextKey struct{}

func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

func TenantIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	tenantID, ok := ctx.Value(tenantContextKey{}).(uuid.UUID)
	return tenantID, ok && tenantID != uuid.Nil
}

func WithPlatformAccess(ctx context.Context) context.Context {
	return context.WithValue(ctx, platformContextKey{}, true)
}

func HasPlatformAccess(ctx context.Context) bool {
	allowed, _ := ctx.Value(platformContextKey{}).(bool)
	return allowed
}

func WithTenantProvisioning(ctx context.Context) context.Context {
	return context.WithValue(ctx, tenantProvisioningContextKey{}, true)
}

func HasTenantProvisioning(ctx context.Context) bool {
	allowed, _ := ctx.Value(tenantProvisioningContextKey{}).(bool)
	return allowed
}
