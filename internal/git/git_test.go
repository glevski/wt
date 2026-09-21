package git

import "testing"

func TestRunEnvPassesEnvironment(t *testing.T) {
	dir := t.TempDir()
	// GIT_CONFIG_COUNT injects config purely through the environment, so a
	// value coming back proves the child saw our variables.
	env := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=wt.probe", "GIT_CONFIG_VALUE_0=seen"}
	if got, err := RunEnv(dir, env, "config", "--get", "wt.probe"); err != nil || got != "seen" {
		t.Errorf("RunEnv config = %q, %v; want seen", got, err)
	}
	if got, _ := Run(dir, "config", "--get", "wt.probe"); got != "" {
		t.Errorf("plain Run leaked the environment: %q", got)
	}
}
