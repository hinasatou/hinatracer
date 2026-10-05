//go:build !windows && !darwin

package i18n

// DetectOSLanguage reads LC_ALL / LC_MESSAGES / LANG / LANGUAGE.
func DetectOSLanguage() string {
	return detectUnixEnvLanguage()
}
