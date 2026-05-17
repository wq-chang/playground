package testenv

import "context"

// SetupServiceForTest exposes setupService to external tests without changing the
// production API surface.
func (te *TestEnv) SetupServiceForTest(
	serviceName,
	configKey string,
	setup func(context.Context) (any, func(), error),
) (any, error) {
	return te.setupService(serviceName, configKey, setup)
}
