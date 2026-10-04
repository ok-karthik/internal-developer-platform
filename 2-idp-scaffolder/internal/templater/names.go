package templater

import "regexp"

// nameRule is the one rule for every user-supplied name the scaffolder turns into a
// folder path or a Kubernetes/Backstage name. Starts with a letter (Kubernetes
// Service names require it); max 40 per name keeps Helm's 53-char release limit and
// DNS labels safe. The joined "<tenant>-<app>-<env>" has its own 63 cap, see maxDerivedName.
var nameRule = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$`)

// maxDerivedName caps len("<tenant>-<app>-<env>"). s3.yaml.tmpl and iam.yaml.tmpl name the
// S3 bucket and the IAM role with that joined string. AWS allows 63 characters for a
// bucket name and 64 for a role name, so 63 is the safe limit for both.
const maxDerivedName = 63

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
	joined := cfg.TenantName + "-" + cfg.AppName + "-" + cfg.Env
	if len(joined) > maxDerivedName {
		return &ValidationError{Field: "tenant-name+app-name+env", Value: joined, Err: ErrNameTooLong}
	}
	return nil
}
