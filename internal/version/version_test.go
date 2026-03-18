package version

import "testing"

func TestFull(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{"default unknown", "dev", "unknown", "dev"},
		{"release with commit", "0.1.0", "abc1234", "0.1.0 (abc1234)"},
		{"empty commit", "1.0.0", "", "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origVersion, origCommit := Version, Commit
			t.Cleanup(func() {
				Version = origVersion
				Commit = origCommit
			})
			Version = tt.version
			Commit = tt.commit
			if got := Full(); got != tt.want {
				t.Errorf("Full() = %q, want %q", got, tt.want)
			}
		})
	}
}
