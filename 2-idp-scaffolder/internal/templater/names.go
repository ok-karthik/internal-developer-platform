package templater

import "regexp"

// nameRule is the one rule for every user-supplied name the scaffolder turns into a
// folder path or a Kubernetes/Backstage name. Starts with a letter (Kubernetes
// Service names require it); max 40 so "<tenant>-<app>-<env>-<cluster>" and Helm's
// 53-char release limit still fit.
var nameRule = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$`)

// checkName returns a *ValidationError naming the field when value breaks nameRule.
func checkName(field, value string) error {
	if !nameRule.MatchString(value) {
		return &ValidationError{Field: field, Value: value, Err: ErrInvalidName}
	}
	return nil
}

// validateTenantNames checks the names onboard-tenant turns into paths and
// Kubernetes names. Field strings equal the CLI flag names.
func validateTenantNames(cfg Config) error {
	if err := checkName("tenant-name", cfg.TenantName); err != nil {
		return err
	}
	for _, o := range cfg.Owners {
		if err := checkName("owner", o); err != nil {
			return err
		}
	}
	return nil
}

// validateServiceNames checks the names add-service turns into paths and
// Kubernetes names. SystemName is optional, so it is only checked when set.
func validateServiceNames(cfg Config) error {
	if err := checkName("tenant-name", cfg.TenantName); err != nil {
		return err
	}
	if err := checkName("app-name", cfg.AppName); err != nil {
		return err
	}
	if err := checkName("env", cfg.Env); err != nil {
		return err
	}
	if cfg.SystemName != "" {
		if err := checkName("system", cfg.SystemName); err != nil {
			return err
		}
	}
	return nil
}
