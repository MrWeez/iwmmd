package auth

// Preset is a non-authoritative OIDC configuration template.
type Preset struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Issuer      string   `json:"issuer,omitempty"`
	Scopes      []string `json:"scopes"`
	GroupsClaim string   `json:"groups_claim"`
	EmailClaim  string   `json:"email_claim"`
	NameClaim   string   `json:"name_claim"`
	Notes       string   `json:"notes,omitempty"`
}

// Presets returns the supported provider templates.
func Presets() []Preset {
	return []Preset{
		{ID: "google", Label: "Google", Issuer: "https://accounts.google.com", Scopes: []string{"openid", "profile", "email"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Google does not emit groups by default. Use an email domain or explicit email allowlist."},
		{ID: "entra", Label: "Microsoft Entra", Issuer: "https://login.microsoftonline.com/{tenant}/v2.0", Scopes: []string{"openid", "profile", "email"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Replace {tenant} with the directory tenant. App roles are preferred over groups for authorization."},
		{ID: "okta", Label: "Okta", Issuer: "https://{yourOktaDomain}/oauth2/default", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Add a groups claim to the authorization server if it is not emitted by default."},
		{ID: "auth0", Label: "Auth0", Issuer: "https://{tenant}.auth0.com/", Scopes: []string{"openid", "profile", "email"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Custom claims require an Auth0 Action."},
		{ID: "gitlab", Label: "GitLab", Issuer: "https://gitlab.com", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Groups depend on the GitLab OIDC configuration."},
		{ID: "keycloak", Label: "Keycloak", Issuer: "https://{host}/realms/{realm}", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Keycloak commonly emits groups or realm_access.roles."},
		{ID: "authentik", Label: "Authentik", Issuer: "https://{host}/application/o/{slug}/", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Use the application slug in the issuer URL."},
		{ID: "authelia", Label: "Authelia", Issuer: "https://{host}", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name"},
		{ID: "pocket-id", Label: "Pocket ID", Issuer: "https://{host}", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name"},
		{ID: "zitadel", Label: "Zitadel", Issuer: "https://{host}", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", Notes: "Zitadel may emit project roles instead of groups."},
		{ID: "generic", Label: "Generic OIDC", Scopes: []string{"openid", "profile", "email", "groups"}, GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name"},
	}
}
