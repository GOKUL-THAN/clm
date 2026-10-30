-- CLM isolated test seed data.
-- Authentication users are intentionally not seeded with tracked credentials;
-- provision test users at runtime through the identity provider.

-- Create the default tenant. Limits match planLimits("enterprise") in
-- internal/saas/tenant/service.go so a plan update does not silently change
-- them.
INSERT INTO tenants (id, name, slug, plan, status, max_certs, max_satellites, max_users, sso_enabled, data_region)
VALUES (
    'a0000000-0000-0000-0000-000000000001',
    'AI Forgers',
    'ai-forgers',
    'enterprise',
    'active',
    100000,
    100,
    500,
    false,
    'ap-south-1'
) ON CONFLICT (slug) DO NOTHING;
